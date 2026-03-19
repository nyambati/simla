package simlaerrors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: assert the error message and that errors.As resolves correctly.
func assertError[T any](t *testing.T, err error, wantMsg string) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, wantMsg, err.Error())
	var target T
	assert.True(t, errors.As(err, &target), "errors.As should resolve to the concrete type")
}

func TestServiceAlreadyExistsError(t *testing.T) {
	err := NewServiceAlreadyExistsError("payments")
	assertError[*ServiceAlreadyExistsError](t, err, "service payments already exists")

	var typed *ServiceAlreadyExistsError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "payments", typed.ServiceName)
}

func TestServiceNotFoundError(t *testing.T) {
	err := NewServiceNotFoundError("orders")
	assertError[*ServiceNotFoundError](t, err, "service orders not found")

	var typed *ServiceNotFoundError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "orders", typed.ServiceName)
}

func TestTimeoutError(t *testing.T) {
	err := NewTimeoutError("invoicer")
	assertError[*TimeoutError](t, err, "request timed out for service invoicer")

	var typed *TimeoutError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "invoicer", typed.ServiceName)
}

func TestConnectionError(t *testing.T) {
	err := NewConnectionError("auth")
	assertError[*ConnectionError](t, err, "failed to connect to service auth")

	var typed *ConnectionError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "auth", typed.ServiceName)
}

func TestServiceInvocationError(t *testing.T) {
	err := NewServiceInvocationError("gateway", 502, "bad upstream")
	assertError[*ServiceInvocationError](t, err, "service gateway returned 502: bad upstream")

	var typed *ServiceInvocationError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "gateway", typed.ServiceName)
	assert.Equal(t, 502, typed.StatusCode)
	assert.Equal(t, "bad upstream", typed.Body)
}

func TestServiceInvocationError_ZeroStatus(t *testing.T) {
	err := NewServiceInvocationError("svc", 0, "")
	assert.Equal(t, "service svc returned 0: ", err.Error())
}

func TestHealthCheckFailedError(t *testing.T) {
	err := NewHeathCheckFailedError("worker", "connection refused")
	assertError[*HealthCheckFailedError](t, err,
		"health check failed for service worker: reason = connection refused")

	var typed *HealthCheckFailedError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "worker", typed.ServiceName)
	assert.Equal(t, "connection refused", typed.Reason)
}

func TestHealthCheckFailedError_EmptyReason(t *testing.T) {
	err := NewHeathCheckFailedError("svc", "")
	assert.Equal(t, "health check failed for service svc: reason = ", err.Error())
}

func TestRuntimeConfigError(t *testing.T) {
	err := NewRuntimeConfigError("missing image")
	assertError[*RuntimeConfigError](t, err, "invalid runtime config: reason = missing image")

	var typed *RuntimeConfigError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "missing image", typed.Reason)
}

func TestRegistryLoadError(t *testing.T) {
	err := NewRegistryLoadError("permission denied")
	assertError[*RegistryLoadError](t, err, "failed to load registry: reason = permission denied")

	var typed *RegistryLoadError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "permission denied", typed.Reason)
}

func TestRegistrySaveError(t *testing.T) {
	err := NewRegistrySaveError("disk full")
	assertError[*RegistrySaveError](t, err, "failed to save registry: reason = disk full")

	var typed *RegistrySaveError
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, "disk full", typed.Reason)
}

// Ensure all error types satisfy the standard error interface at compile time.
var (
	_ error = (*ServiceAlreadyExistsError)(nil)
	_ error = (*ServiceNotFoundError)(nil)
	_ error = (*TimeoutError)(nil)
	_ error = (*ConnectionError)(nil)
	_ error = (*ServiceInvocationError)(nil)
	_ error = (*HealthCheckFailedError)(nil)
	_ error = (*RuntimeConfigError)(nil)
	_ error = (*RegistryLoadError)(nil)
	_ error = (*RegistrySaveError)(nil)
)
