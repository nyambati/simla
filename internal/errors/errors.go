package simlaerrors

import "fmt"

type ServiceAlreadyExistsError struct {
	ServiceName string
}

func NewServiceAlreadyExistsError(name string) error {
	return &ServiceAlreadyExistsError{ServiceName: name}
}

func (e *ServiceAlreadyExistsError) Error() string {
	return fmt.Sprintf("service %s already exists", e.ServiceName)
}

type ServiceNotFoundError struct {
	ServiceName string
}

func NewServiceNotFoundError(name string) error {
	return &ServiceNotFoundError{ServiceName: name}
}

func (e *ServiceNotFoundError) Error() string {
	return fmt.Sprintf("service %s not found", e.ServiceName)
}

type TimeoutError struct {
	ServiceName string
}

func NewTimeoutError(name string) error {
	return &TimeoutError{ServiceName: name}
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("request timed out for service %s", e.ServiceName)
}

type ConnectionError struct {
	ServiceName string
}

func NewConnectionError(name string) error {
	return &ConnectionError{ServiceName: name}
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("failed to connect to service %s", e.ServiceName)
}

type ServiceInvocationError struct {
	ServiceName string
	StatusCode  int
	Body        string
}

func NewServiceInvocationError(name string, status int, body string) error {
	return &ServiceInvocationError{ServiceName: name, StatusCode: status, Body: body}
}

func (e *ServiceInvocationError) Error() string {
	return fmt.Sprintf("service %s returned %d: %s", e.ServiceName, e.StatusCode, e.Body)
}

// Health check error
type HealthCheckFailedError struct {
	ServiceName string
	Reason      string
}

func NewHeathCheckFailedError(name string, reason string) error {
	return &HealthCheckFailedError{ServiceName: name}
}

func (e *HealthCheckFailedError) Error() string {
	return fmt.Sprintf("health check failed for service %s: reason = %s", e.ServiceName, e.Reason)
}

// Runtime errors
type RuntimeConfigError struct {
	Reason string
}

func (e *RuntimeConfigError) Error() string {
	return fmt.Sprintf("invalid runtime config: reason = %s", e.Reason)
}

func NewRuntimeConfigError(reason string) error {
	return &RuntimeConfigError{Reason: reason}
}

// Registry errors
type RegistryLoadError struct {
	Reason string
}

func (e *RegistryLoadError) Error() string {
	return fmt.Sprintf("failed to load registry: reason = %s", e.Reason)
}

func NewRegistryLoadError(reason string) error {
	return &RegistryLoadError{Reason: reason}
}

type RegistrySaveError struct {
	Reason string
}

func (e *RegistrySaveError) Error() string {
	return fmt.Sprintf("failed to save registry: reason = %s", e.Reason)
}

func NewRegistrySaveError(reason string) error {
	return &RegistrySaveError{Reason: reason}
}
