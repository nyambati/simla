//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_workflow.go -package=mocks ExecutorInterface

package workflow

import (
	"context"
	"time"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

// ExecutionStatus represents the overall status of a workflow execution.
type ExecutionStatus string

const (
	ExecutionStatusRunning   ExecutionStatus = "RUNNING"
	ExecutionStatusSucceeded ExecutionStatus = "SUCCEEDED"
	ExecutionStatusFailed    ExecutionStatus = "FAILED"
	ExecutionStatusTimedOut  ExecutionStatus = "TIMED_OUT"
	ExecutionStatusAborted   ExecutionStatus = "ABORTED"
)

// Standard AWS Step Functions error names used in Retry/Catch matchers.
const (
	ErrAll                             = "States.ALL"
	ErrTimeout                         = "States.Timeout"
	ErrTaskFailed                      = "States.TaskFailed"
	ErrPermissions                     = "States.Permissions"
	ErrResultPathNull                  = "States.ResultPathMatchFailure"
	ErrBranchFailed                    = "States.BranchFailed"
	ErrNoChoiceMatched                 = "States.NoChoiceMatched"
	ErrIntrinsicFailure                = "States.IntrinsicFailure"
	ErrExceedToleratedFailureThreshold = "States.ExceedToleratedFailureThreshold"
)

// ExecutorInterface is the primary interface for running workflows.
type ExecutorInterface interface {
	// Execute runs the named workflow with the given JSON input and returns the
	// final JSON output. workflowName must match a key under Config.Workflows.
	Execute(ctx context.Context, workflowName string, input []byte) ([]byte, error)
}

// Execution holds the runtime state of a single workflow run.
type Execution struct {
	ID           string
	WorkflowName string
	Status       ExecutionStatus
	Input        []byte
	Output       []byte
	StartedAt    time.Time
	StoppedAt    time.Time
	Error        string
	Cause        string
}

// Executor is the concrete implementation of ExecutorInterface.
type Executor struct {
	config    *config.Config
	scheduler scheduler.SchedulerInterface
	logger    *logrus.Entry
}

// stateResult carries the JSON data passing between states plus the name of
// the next state to transition to (empty string means terminal).
type stateResult struct {
	output    []byte
	nextState string
	end       bool
}
