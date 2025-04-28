package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/aws/aws-lambda-go/events"
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
		logger := g.logger.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"method": r.Method,
		})

		if r.Method != route.Method {
			logger.Warnf("unsuported method: expected=%s, got=%s", route.Method, r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("recieved request for service")

		defer r.Body.Close()

		body, err := g.buildAPIGatewayEvent(r)
		if err != nil {
			http.Error(w, "failed to build api gateway request", http.StatusBadRequest)
			return
		}
		// Prepare context with timeout
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Set service name in context (for tracing)
		ctx = context.WithValue(ctx, "service", route.Service)

		// Invoke service through scheduler
		response, err := g.scheduler.Invoke(ctx, route.Service, body)
		if err != nil {
			g.logger.WithError(err).Error("failed to invoke service")
			http.Error(w, "failed to process request", http.StatusBadGateway)
			return
		}

		// Success - write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, bytes.NewReader(response))

		duration := time.Since(start)
		g.logger.WithField("duration", duration).Info("successfully routed request")
	}
}

func (g *APIGateway) createHttpServer(ctx context.Context) error {
	server := &http.Server{
		Addr:    ":" + g.config.Port,
		Handler: g.router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			g.logger.WithError(err).Fatal("failed to start gateway")
		}
	}()

	<-ctx.Done()
	g.logger.Info("shutting now API gateway")
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

func (g *APIGateway) buildAPIGatewayEvent(r *http.Request) ([]byte, error) {
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
		Headers:        extractHeaders(r),
		Cookies:        extractCookies(r),
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RouteKey:   routeKey,
			AccountID:  "012345678901",
			Stage:      g.config.Stage,
			Time:       time.Now().String(),
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
		Body:            string(body),
		IsBase64Encoded: true,
	}

	bytes, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

func extractCookies(r *http.Request) []string {
	cookies := []string{}
	for _, cookie := range r.Cookies() {
		cookies = append(cookies, cookie.Value)
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

		next.ServeHTTP(w, r) // Call the next handler

		duration := time.Since(start)

		g.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"remoteAddr": r.RemoteAddr,
			"duration":   duration,
		}).Info("handled request")
	})
}

func formatRoutePath(stage, routePath string) string {
	return fmt.Sprintf("%s/%s", stage, routePath)
}
