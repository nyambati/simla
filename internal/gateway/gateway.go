package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/registry"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

func NewAPIGateway(config *config.Config, registry registry.ServiceRegistryInterface, logger *logrus.Logger) GatewayInterface {
	scheduler := scheduler.NewScheduler(config, registry, logger.WithField("component", "scheduler"))
	return &APIGateway{
		config:    &config.APIGateway,
		scheduler: scheduler,
		logger:    logger.WithField("component", "gateway"),
		router:    mux.NewRouter(),
	}
}

func (g *APIGateway) Start(ctx context.Context) error {
	g.logger.Infof("starting gateway on port %s", g.config.Port)
	g.router.HandleFunc(filepath.Join("/", g.config.Stage, "/health"), g.handleHealthCheck()).Methods(http.MethodGet)
	g.router.Use(g.loggingMiddleware)

	for _, route := range g.config.Routes {
		rPath := filepath.Join("/", g.config.Stage, route.Path)
		fields := logrus.Fields{
			"method":  route.Method,
			"path":    rPath,
			"service": route.Service,
		}
		g.logger.WithFields(fields).Info("registering route")
		g.router.Methods(route.Method).Path(rPath).HandlerFunc(g.handleRequest(route))
	}
	return g.createHttpServer(ctx)
}

func (g *APIGateway) handleRequest(route config.Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate a request ID and attach it to every inbound request so it
		// can be correlated across logs, the Lambda event, and the response.
		requestID := uuid.NewString()
		logger := g.logger.WithFields(logrus.Fields{
			"path":       r.URL.Path,
			"method":     r.Method,
			"request_id": requestID,
		})

		if r.Method != route.Method {
			logger.Warnf("unsupported method: expected=%s, got=%s", route.Method, r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("received request for service")

		defer r.Body.Close()

		body, err := g.buildAPIGatewayEvent(r, requestID)
		if err != nil {
			logger.WithError(err).Error("failed to build api gateway event")
			http.Error(w, "failed to build api gateway request", http.StatusBadRequest)
			return
		}

		// Prepare context with timeout and service name for downstream tracing.
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, "service", route.Service)

		// Invoke the Lambda through the scheduler.
		response, err := g.scheduler.Invoke(ctx, route.Service, body)
		if err != nil {
			logger.WithError(err).Error("failed to invoke service")
			writeJSONError(w, http.StatusBadGateway, err.Error(), requestID)
			return
		}

		// Attempt to interpret the response as a full APIGatewayV2HTTPResponse.
		// If the Lambda returned one, honour its status code, headers, and body.
		// Otherwise fall back to a plain 200 with the raw bytes as the body.
		if err := writePassthroughResponse(w, response, requestID, logger); err != nil {
			// Not an APIGatewayV2HTTPResponse — write raw bytes at 200.
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", requestID)
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, bytes.NewReader(response))
		}

		logger.WithField("duration", time.Since(start)).Info("successfully routed request")
	}
}

// writePassthroughResponse tries to unmarshal body as events.APIGatewayV2HTTPResponse.
// It returns an error if the body is not a valid response structure (so the
// caller can fall back to a raw write). A valid response must have a non-zero
// StatusCode field.
func writePassthroughResponse(w http.ResponseWriter, body []byte, requestID string, logger *logrus.Entry) error {
	var resp events.APIGatewayV2HTTPResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	if resp.StatusCode == 0 {
		return fmt.Errorf("not an APIGatewayV2HTTPResponse: StatusCode is 0")
	}

	// Copy single-value headers from the Lambda response.
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	// Copy multi-value headers (these take precedence over single-value for
	// the same key per the AWS spec).
	for k, vs := range resp.MultiValueHeaders {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	// Forward cookies the Lambda set via the Cookies field.
	for _, c := range resp.Cookies {
		w.Header().Add("Set-Cookie", c)
	}

	w.Header().Set("X-Request-ID", requestID)

	// Default content-type if the Lambda didn't set one.
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(resp.StatusCode)

	if resp.Body != "" {
		var bodyBytes []byte
		if resp.IsBase64Encoded {
			var err error
			bodyBytes, err = base64.StdEncoding.DecodeString(resp.Body)
			if err != nil {
				// Headers are already written; log the failure so it is
				// observable, but do not surface it as an error to the caller
				// (WriteHeader has already been called, so no fallback is possible).
				logger.WithError(err).Error("failed to base64-decode Lambda response body")
				return nil
			}
		} else {
			bodyBytes = []byte(resp.Body)
		}
		_, _ = w.Write(bodyBytes)
	}

	return nil
}

// writeJSONError writes a structured JSON error body with the given status code.
func writeJSONError(w http.ResponseWriter, statusCode int, message string, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(statusCode)
	body, _ := json.Marshal(map[string]string{"error": message, "requestId": requestID})
	_, _ = w.Write(body)
}

func (g *APIGateway) createHttpServer(ctx context.Context) error {
	server := &http.Server{
		Addr:    ":" + g.config.Port,
		Handler: g.router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			g.logger.WithError(err).Fatal("failed to start gateway")
		}
	}()

	<-ctx.Done()
	g.logger.Info("shutting down API gateway")
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

func (g *APIGateway) buildAPIGatewayEvent(r *http.Request, requestID string) ([]byte, error) {
	routeKey := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	event := &events.APIGatewayV2HTTPRequest{
		Version:        "2.0",
		RouteKey:       routeKey,
		RawPath:        r.URL.Path,
		RawQueryString: r.URL.RawQuery,
		Headers:        extractHeaders(r, requestID),
		Cookies:        extractCookies(r),
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RouteKey:   routeKey,
			AccountID:  "012345678901",
			Stage:      g.config.Stage,
			RequestID:  requestID,
			Time:       time.Now().Format(time.RFC3339),
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{},
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:    r.Method,
				Path:      r.URL.Path,
				Protocol:  r.Proto,
				SourceIP:  r.RemoteAddr,
				UserAgent: r.UserAgent(),
			},
			Authentication: events.APIGatewayV2HTTPRequestContextAuthentication{},
		},
		Body:            bodyString(body),
		IsBase64Encoded: !utf8.Valid(body),
	}

	return json.Marshal(event)
}

// extractHeaders collects the first value of every request header and injects
// the X-Request-ID so Lambdas can read it from event.Headers.
func extractHeaders(r *http.Request, requestID string) map[string]string {
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	headers["X-Request-ID"] = requestID
	return headers
}

func extractCookies(r *http.Request) []string {
	cookies := []string{}
	for _, cookie := range r.Cookies() {
		cookies = append(cookies, cookie.Name+"="+cookie.Value)
	}
	return cookies
}

func (g *APIGateway) handleHealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}
}

func (g *APIGateway) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		g.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"remoteAddr": r.RemoteAddr,
			"duration":   time.Since(start),
		}).Info("handled request")
	})
}

// bodyString returns body as a plain string when it is valid UTF-8, or as a
// base64-encoded string otherwise.
func bodyString(body []byte) string {
	if utf8.Valid(body) {
		return string(body)
	}
	return base64.StdEncoding.EncodeToString(body)
}
