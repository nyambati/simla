package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/gorilla/mux"
	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestGateway builds an APIGateway wired to a mock scheduler for unit tests.
// It registers the supplied routes on the returned router so callers can drive
// requests through httptest without starting a real HTTP server.
func newTestGateway(t *testing.T, routes []config.Route, sched *mocks.MockSchedulerInterface) (*APIGateway, *mux.Router) {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	gw := &APIGateway{
		config: &config.APIGateway{
			Port:   "8080",
			Stage:  "v1",
			Routes: routes,
		},
		scheduler: sched,
		logger:    logger.WithField("component", "gateway"),
		router:    mux.NewRouter(),
	}

	// Register the health check.
	gw.router.HandleFunc("/v1/health", gw.handleHealthCheck()).Methods(http.MethodGet)
	gw.router.Use(gw.loggingMiddleware)

	// Register each route.
	for _, route := range routes {
		r := route
		gw.router.Methods(r.Method).Path("/v1/" + r.Path).HandlerFunc(gw.handleRequest(r))
	}

	return gw, gw.router
}

// ------- health check -------------------------------------------------------

func TestHandleHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, router := newTestGateway(t, nil, sched)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}

// ------- request ID ---------------------------------------------------------

func TestRequestID_PresentOnSuccessResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "payments", Method: http.MethodGet, Service: "payments"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	sched.EXPECT().
		Invoke(gomock.Any(), "payments", gomock.Any()).
		Return([]byte(`"ok"`), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/payments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEmpty(t, w.Header().Get("X-Request-ID"), "X-Request-ID must be set on success response")
}

func TestRequestID_PresentOnErrorResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "payments", Method: http.MethodGet, Service: "payments"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	sched.EXPECT().
		Invoke(gomock.Any(), "payments", gomock.Any()).
		Return(nil, errors.New("downstream error"))

	req := httptest.NewRequest(http.MethodGet, "/v1/payments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"), "X-Request-ID must be set on error response")

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, body["requestId"], w.Header().Get("X-Request-ID"))
}

func TestRequestID_InjectedIntoEventHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "echo", Method: http.MethodPost, Service: "echo"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	var capturedPayload []byte
	sched.EXPECT().
		Invoke(gomock.Any(), "echo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, payload []byte) ([]byte, error) {
			capturedPayload = payload
			return []byte(`"captured"`), nil
		})

	req := httptest.NewRequest(http.MethodPost, "/v1/echo", strings.NewReader(`{"hello":"world"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var event events.APIGatewayV2HTTPRequest
	require.NoError(t, json.Unmarshal(capturedPayload, &event))

	rid := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, rid)
	assert.Equal(t, rid, event.Headers["X-Request-ID"], "request ID must be forwarded into event.Headers")
	assert.Equal(t, rid, event.RequestContext.RequestID, "request ID must be set on event.RequestContext.RequestID")
}

// ------- raw response passthrough (scalar / non-APIGateway response) --------

func TestHandleRequest_RawResponsePassthrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "greet", Method: http.MethodGet, Service: "greet"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	sched.EXPECT().
		Invoke(gomock.Any(), "greet", gomock.Any()).
		Return([]byte(`"Hello, Simla!"`), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/greet", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, `"Hello, Simla!"`, w.Body.String())
}

// ------- APIGatewayV2HTTPResponse passthrough --------------------------------

func TestHandleRequest_PassthroughCustomStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "resource", Method: http.MethodPost, Service: "svc"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	lambdaResp := events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"id":"abc"}`,
	}
	respBytes, _ := json.Marshal(lambdaResp)

	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		Return(respBytes, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/resource", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, `{"id":"abc"}`, w.Body.String())
}

func TestHandleRequest_PassthroughMultiValueHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "multi", Method: http.MethodGet, Service: "svc"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	lambdaResp := events.APIGatewayV2HTTPResponse{
		StatusCode:        http.StatusOK,
		MultiValueHeaders: map[string][]string{"X-Custom": {"a", "b"}},
		Body:              "ok",
	}
	respBytes, _ := json.Marshal(lambdaResp)

	sched.EXPECT().Invoke(gomock.Any(), "svc", gomock.Any()).Return(respBytes, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/multi", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"a", "b"}, w.Header()["X-Custom"])
}

func TestHandleRequest_PassthroughCookies(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "login", Method: http.MethodPost, Service: "auth"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	lambdaResp := events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Cookies:    []string{"session=abc123; HttpOnly", "theme=dark"},
		Body:       "logged in",
	}
	respBytes, _ := json.Marshal(lambdaResp)

	sched.EXPECT().Invoke(gomock.Any(), "auth", gomock.Any()).Return(respBytes, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/login", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	setCookies := w.Header()["Set-Cookie"]
	assert.Contains(t, setCookies, "session=abc123; HttpOnly")
	assert.Contains(t, setCookies, "theme=dark")
}

func TestHandleRequest_PassthroughBase64Body(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "binary", Method: http.MethodGet, Service: "bin"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	raw := []byte{0x89, 0x50, 0x4e, 0x47} // PNG magic bytes
	lambdaResp := events.APIGatewayV2HTTPResponse{
		StatusCode:      http.StatusOK,
		IsBase64Encoded: true,
		Headers:         map[string]string{"Content-Type": "image/png"},
		Body:            base64.StdEncoding.EncodeToString(raw),
	}
	respBytes, _ := json.Marshal(lambdaResp)

	sched.EXPECT().Invoke(gomock.Any(), "bin", gomock.Any()).Return(respBytes, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/binary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, raw, w.Body.Bytes(), "base64-encoded body must be decoded before writing")
}

// ------- error paths --------------------------------------------------------

func TestHandleRequest_SchedulerError_Returns502(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	route := config.Route{Path: "boom", Method: http.MethodGet, Service: "boom"}
	_, router := newTestGateway(t, []config.Route{route}, sched)

	sched.EXPECT().
		Invoke(gomock.Any(), "boom", gomock.Any()).
		Return(nil, errors.New("container not healthy"))

	req := httptest.NewRequest(http.MethodGet, "/v1/boom", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Contains(t, body["error"], "container not healthy")
	assert.NotEmpty(t, body["requestId"])
}

// ------- helper functions ---------------------------------------------------

func TestExtractCookies_NameValueFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})

	cookies := extractCookies(req)

	assert.Contains(t, cookies, "session=abc")
	assert.Contains(t, cookies, "theme=dark")
}

func TestBodyString_UTF8_NotEncoded(t *testing.T) {
	input := []byte(`{"key":"value"}`)
	result := bodyString(input)
	assert.Equal(t, `{"key":"value"}`, result)
}

func TestBodyString_Binary_Base64Encoded(t *testing.T) {
	input := []byte{0x00, 0x01, 0x02, 0xff}
	result := bodyString(input)
	assert.Equal(t, base64.StdEncoding.EncodeToString(input), result)
}

func TestBuildAPIGatewayEvent_Fields(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	gw := &APIGateway{
		config: &config.APIGateway{Stage: "v1"},
		logger: logger.WithField("component", "gateway"),
	}

	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/greet?foo=bar", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "tok", Value: "xyz"})

	rawEvent, err := gw.buildAPIGatewayEvent(req, "test-request-id")
	require.NoError(t, err)

	var event events.APIGatewayV2HTTPRequest
	require.NoError(t, json.Unmarshal(rawEvent, &event))

	assert.Equal(t, "2.0", event.Version)
	assert.Equal(t, "POST /v1/greet", event.RouteKey)
	assert.Equal(t, "/v1/greet", event.RawPath)
	assert.Equal(t, "foo=bar", event.RawQueryString)
	assert.Equal(t, `{"name":"test"}`, event.Body)
	assert.False(t, event.IsBase64Encoded)
	assert.Equal(t, "test-request-id", event.RequestContext.RequestID)
	assert.Equal(t, "v1", event.RequestContext.Stage)
	assert.Equal(t, http.MethodPost, event.RequestContext.HTTP.Method)
	assert.Equal(t, "test-request-id", event.Headers["X-Request-ID"])
	assert.Contains(t, event.Cookies, "tok=xyz")
}

func TestBuildAPIGatewayEvent_BinaryBody_IsBase64(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	gw := &APIGateway{
		config: &config.APIGateway{Stage: "v1"},
		logger: logger.WithField("component", "gateway"),
	}

	binaryBody := []byte{0x89, 0x50, 0x4e, 0x47}
	req := httptest.NewRequest(http.MethodPost, "/v1/upload", strings.NewReader(string(binaryBody)))

	rawEvent, err := gw.buildAPIGatewayEvent(req, "rid")
	require.NoError(t, err)

	var event events.APIGatewayV2HTTPRequest
	require.NoError(t, json.Unmarshal(rawEvent, &event))

	assert.True(t, event.IsBase64Encoded)
	decoded, err := base64.StdEncoding.DecodeString(event.Body)
	require.NoError(t, err)
	assert.Equal(t, binaryBody, decoded)
}
