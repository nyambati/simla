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
	return &HealthCheckFailedError{ServiceName: name, Reason: reason}
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

// Workflow errors

type WorkflowNotFoundError struct {
	WorkflowName string
}

func NewWorkflowNotFoundError(name string) error {
	return &WorkflowNotFoundError{WorkflowName: name}
}

func (e *WorkflowNotFoundError) Error() string {
	return fmt.Sprintf("workflow %s not found", e.WorkflowName)
}

type WorkflowStateError struct {
	WorkflowName string
	StateName    string
	Cause        string
}

func NewWorkflowStateError(workflow, state, cause string) error {
	return &WorkflowStateError{WorkflowName: workflow, StateName: state, Cause: cause}
}

func (e *WorkflowStateError) Error() string {
	return fmt.Sprintf("workflow %s failed at state %s: %s", e.WorkflowName, e.StateName, e.Cause)
}

type WorkflowExecutionError struct {
	WorkflowName string
	Error_       string
	Cause        string
}

func NewWorkflowExecutionError(workflow, errName, cause string) error {
	return &WorkflowExecutionError{WorkflowName: workflow, Error_: errName, Cause: cause}
}

func (e *WorkflowExecutionError) Error() string {
	return fmt.Sprintf("workflow %s execution failed (%s): %s", e.WorkflowName, e.Error_, e.Cause)
}

type WorkflowTimeoutError struct {
	WorkflowName string
	StateName    string
}

func NewWorkflowTimeoutError(workflow, state string) error {
	return &WorkflowTimeoutError{WorkflowName: workflow, StateName: state}
}

func (e *WorkflowTimeoutError) Error() string {
	if e.StateName != "" {
		return fmt.Sprintf("workflow %s timed out at state %s", e.WorkflowName, e.StateName)
	}
	return fmt.Sprintf("workflow %s timed out", e.WorkflowName)
}
