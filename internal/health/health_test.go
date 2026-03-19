package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestChecker builds a HealthChecker pointed at the given base URL.
// It uses a short timeout to keep tests fast.
func newTestChecker(timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		client:  &http.Client{Timeout: 2 * time.Second},
		logger:  logrus.NewEntry(logrus.New()),
		timeout: timeout,
	}
}

// serviceCtx injects a service name into ctx (required by IsHealthy /
// WaitForHealthy).
func serviceCtx(name string) context.Context {
	return context.WithValue(context.Background(), "service", name)
}

// testServer creates an httptest.Server that responds with the given status
// code. The caller must call srv.Close() when done.
func testServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
}

// overrideEndpoint replaces healthCheckEndpoint so that IsHealthy/WaitForHealthy
// hit the supplied httptest.Server instead of localhost:<port>.
// It derives the host from the server URL and constructs a valid format string
// that accepts the (ignored) port argument.
func overrideEndpoint(t *testing.T, srv *httptest.Server) {
	t.Helper()
	original := healthCheckEndpoint
	// Replace with the server's full URL; append /%s so Sprintf is happy with
	// the port arg (the extra path segment is harmless for test handlers).
	healthCheckEndpoint = srv.URL + "/%s"
	t.Cleanup(func() { healthCheckEndpoint = original })
}

// ── IsHealthy ────────────────────────────────────────────────────────────────

func TestIsHealthy_Returns_True_On_2xx(t *testing.T) {
	for _, code := range []int{200, 201, 204, 299} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := testServer(code)
			defer srv.Close()

			// Point the endpoint directly at the test server; port is ignored.
			original := healthCheckEndpoint
			healthCheckEndpoint = srv.URL
			defer func() { healthCheckEndpoint = original }()

			hc := newTestChecker(5 * time.Second)
			svc := &registry.Service{Port: 0}
			ok, err := hc.IsHealthy(serviceCtx("svc"), svc)
			require.NoError(t, err)
			assert.True(t, ok)
		})
	}
}

func TestIsHealthy_Returns_False_On_Non2xx(t *testing.T) {
	for _, code := range []int{400, 404, 500, 503} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := testServer(code)
			defer srv.Close()

			original := healthCheckEndpoint
			healthCheckEndpoint = srv.URL
			defer func() { healthCheckEndpoint = original }()

			hc := newTestChecker(5 * time.Second)
			svc := &registry.Service{Port: 0}
			ok, err := hc.IsHealthy(serviceCtx("svc"), svc)
			assert.False(t, ok)
			require.Error(t, err)

			var hcErr *simlaerrors.HealthCheckFailedError
			assert.ErrorAs(t, err, &hcErr)
		})
	}
}

func TestIsHealthy_MissingContextKey_ReturnsError(t *testing.T) {
	hc := newTestChecker(5 * time.Second)
	svc := &registry.Service{Port: 9999}
	// context has no "service" key
	ok, err := hc.IsHealthy(context.Background(), svc)
	assert.False(t, ok)
	require.Error(t, err)

	var hcErr *simlaerrors.HealthCheckFailedError
	require.ErrorAs(t, err, &hcErr)
	assert.Equal(t, "unknown", hcErr.ServiceName)
}

func TestIsHealthy_ConnectionRefused_ReturnsError(t *testing.T) {
	hc := newTestChecker(500 * time.Millisecond)
	// Port 1 is almost certainly not listening.
	svc := &registry.Service{Port: 1}
	ok, err := hc.IsHealthy(serviceCtx("svc"), svc)
	assert.False(t, ok)
	require.Error(t, err)
}

func TestIsHealthy_CancelledContext_ReturnsError(t *testing.T) {
	// Server that hangs indefinitely.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	original := healthCheckEndpoint
	healthCheckEndpoint = srv.URL
	defer func() { healthCheckEndpoint = original }()

	ctx, cancel := context.WithCancel(serviceCtx("svc"))
	cancel() // cancel immediately

	hc := newTestChecker(5 * time.Second)
	svc := &registry.Service{Port: 0}
	ok, err := hc.IsHealthy(ctx, svc)
	assert.False(t, ok)
	require.Error(t, err)
}

// ── WaitForHealthy ────────────────────────────────────────────────────────────

func TestWaitForHealthy_SucceedsWhenHealthy(t *testing.T) {
	srv := testServer(http.StatusOK)
	defer srv.Close()

	original := healthCheckEndpoint
	healthCheckEndpoint = srv.URL
	defer func() { healthCheckEndpoint = original }()

	hc := newTestChecker(5 * time.Second)
	svc := &registry.Service{Port: 0}
	err := hc.WaitForHealthy(serviceCtx("svc"), svc)
	assert.NoError(t, err)
}

func TestWaitForHealthy_TimesOutWhenNeverHealthy(t *testing.T) {
	srv := testServer(http.StatusServiceUnavailable)
	defer srv.Close()

	original := healthCheckEndpoint
	healthCheckEndpoint = srv.URL
	defer func() { healthCheckEndpoint = original }()

	// Very short timeout so the test finishes quickly.
	hc := newTestChecker(600 * time.Millisecond)
	svc := &registry.Service{Port: 0}
	err := hc.WaitForHealthy(serviceCtx("svc"), svc)
	require.Error(t, err)

	var timeoutErr *simlaerrors.TimeoutError
	assert.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "svc", timeoutErr.ServiceName)
}

func TestWaitForHealthy_MissingContextKey_ReturnsTimeoutError(t *testing.T) {
	hc := newTestChecker(5 * time.Second)
	svc := &registry.Service{Port: 9999}
	err := hc.WaitForHealthy(context.Background(), svc) // no "service" key
	require.Error(t, err)

	var timeoutErr *simlaerrors.TimeoutError
	assert.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "unknown", timeoutErr.ServiceName)
}

func TestWaitForHealthy_ContextCancelled_ReturnsTimeoutError(t *testing.T) {
	srv := testServer(http.StatusServiceUnavailable)
	defer srv.Close()

	original := healthCheckEndpoint
	healthCheckEndpoint = srv.URL
	defer func() { healthCheckEndpoint = original }()

	ctx, cancel := context.WithCancel(serviceCtx("svc"))
	// Cancel after a short delay so at least one ticker fires.
	time.AfterFunc(150*time.Millisecond, cancel)

	hc := newTestChecker(30 * time.Second)
	svc := &registry.Service{Port: 0}
	err := hc.WaitForHealthy(ctx, svc)
	require.Error(t, err)

	var timeoutErr *simlaerrors.TimeoutError
	assert.ErrorAs(t, err, &timeoutErr)
}

func TestWaitForHealthy_EventuallyHealthy(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	original := healthCheckEndpoint
	healthCheckEndpoint = srv.URL
	defer func() { healthCheckEndpoint = original }()

	hc := &HealthChecker{
		client:  &http.Client{Timeout: 2 * time.Second},
		logger:  logrus.NewEntry(logrus.New()),
		timeout: 5 * time.Second,
	}
	svc := &registry.Service{Port: 0}
	err := hc.WaitForHealthy(serviceCtx("svc"), svc)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, callCount, 3)
}

// ── NewHealthChecker ──────────────────────────────────────────────────────────

func TestNewHealthChecker_ImplementsInterface(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	hc := NewHealthChecker(logger)
	require.NotNil(t, hc)
	// Verify the returned value satisfies the interface at compile time.
	var _ HealthCheckerInterface = hc
}
