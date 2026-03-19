package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nyambati/simla/internal/config"
	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

// NewExecutor creates an Executor that resolves workflow definitions from cfg
// and invokes services via sched.
func NewExecutor(cfg *config.Config, sched scheduler.SchedulerInterface, logger *logrus.Entry) ExecutorInterface {
	return &Executor{
		config:    cfg,
		scheduler: sched,
		logger:    logger.WithField("component", "workflow"),
	}
}

// Execute runs the named state machine with the provided JSON input.
// It returns the final JSON output of the workflow on success.
func (e *Executor) Execute(ctx context.Context, workflowName string, input []byte) ([]byte, error) {
	sm, ok := e.config.GetWorkflow(ctx, workflowName)
	if !ok {
		return nil, simlaerrors.NewWorkflowNotFoundError(workflowName)
	}

	execID := uuid.NewString()
	logger := e.logger.WithFields(logrus.Fields{
		"workflow":     workflowName,
		"execution_id": execID,
	})
	logger.Info("starting workflow execution")

	exec := &Execution{
		ID:           execID,
		WorkflowName: workflowName,
		Status:       ExecutionStatusRunning,
		Input:        input,
		StartedAt:    time.Now(),
	}

	output, err := e.runMachine(ctx, sm, input, logger)
	exec.StoppedAt = time.Now()
	if err != nil {
		exec.Status = ExecutionStatusFailed
		exec.Error = err.Error()
		logger.WithError(err).Error("workflow execution failed")
		return nil, err
	}

	exec.Status = ExecutionStatusSucceeded
	exec.Output = output
	logger.WithField("duration", exec.StoppedAt.Sub(exec.StartedAt)).Info("workflow execution succeeded")
	return output, nil
}

// runMachine drives the state machine loop for a single StateMachine (which
// may be a top-level workflow or a branch inside a Parallel state).
func (e *Executor) runMachine(ctx context.Context, sm *config.StateMachine, input []byte, logger *logrus.Entry) ([]byte, error) {
	if sm.StartAt == "" {
		return nil, fmt.Errorf("state machine %q has no StartAt", sm.Name)
	}

	currentState := sm.StartAt
	data := input

	for {
		stateDef, ok := sm.States[currentState]
		if !ok {
			return nil, simlaerrors.NewWorkflowStateError(sm.Name, currentState, "state not found in definition")
		}

		logger := logger.WithField("state", currentState)
		logger.Infof("entering state (type=%s)", stateDef.Type)

		result, err := e.executeState(ctx, sm.Name, currentState, &stateDef, data, logger)
		if err != nil {
			return nil, err
		}

		data = result.output

		if result.end || stateDef.End {
			logger.Info("reached terminal state")
			return data, nil
		}

		if result.nextState == "" {
			return nil, simlaerrors.NewWorkflowStateError(sm.Name, currentState, "no Next defined and state is not terminal")
		}

		currentState = result.nextState
	}
}

// executeState dispatches to the appropriate state handler.
func (e *Executor) executeState(
	ctx context.Context,
	workflowName, stateName string,
	state *config.State,
	input []byte,
	logger *logrus.Entry,
) (*stateResult, error) {
	switch config.StateType(state.Type) {
	case config.StateTypeTask:
		return e.executeTask(ctx, workflowName, stateName, state, input, logger)
	case config.StateTypePass:
		return e.executePass(state, input)
	case config.StateTypeChoice:
		return e.executeChoice(workflowName, stateName, state, input)
	case config.StateTypeParallel:
		return e.executeParallel(ctx, workflowName, stateName, state, input, logger)
	case config.StateTypeWait:
		return e.executeWait(ctx, workflowName, stateName, state, input)
	case config.StateTypeSucceed:
		return &stateResult{output: input, end: true}, nil
	case config.StateTypeFail:
		cause := state.Cause
		if state.CausePath != "" {
			if raw, err := applyPath(input, state.CausePath); err == nil {
				cause = strings.Trim(string(raw), `"`)
			}
		}
		return nil, simlaerrors.NewWorkflowExecutionError(workflowName, state.Error, cause)
	default:
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
			fmt.Sprintf("unknown state type %q", state.Type))
	}
}

// ---------------------------------------------------------------------------
// Task state
// ---------------------------------------------------------------------------

func (e *Executor) executeTask(
	ctx context.Context,
	workflowName, stateName string,
	state *config.State,
	input []byte,
	logger *logrus.Entry,
) (*stateResult, error) {
	if state.Resource == "" {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName, "Task state has no Resource (service name)")
	}

	// Apply InputPath to narrow the data sent to the service.
	effective, err := applyPath(input, state.InputPath)
	if err != nil {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName, fmt.Sprintf("InputPath error: %v", err))
	}

	// Build a timeout context if TimeoutSeconds is set.
	invokeCtx := ctx
	if state.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(state.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// Put service name into context (required by health checker and router).
	invokeCtx = context.WithValue(invokeCtx, "service", state.Resource)

	var taskOutput []byte
	var taskErr error

	taskOutput, taskErr = e.invokeWithRetry(invokeCtx, workflowName, stateName, state, effective, logger)

	if taskErr != nil {
		// Try Catch blocks.
		if len(state.Catch) > 0 {
			next, catchOutput, matched := e.tryCatch(state.Catch, taskErr, input)
			if matched {
				logger.WithError(taskErr).Infof("error caught, transitioning to %s", next)
				return &stateResult{output: catchOutput, nextState: next}, nil
			}
		}
		return nil, taskErr
	}

	// Apply OutputPath to the raw task response.
	filtered, err := applyPath(taskOutput, state.OutputPath)
	if err != nil {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName, fmt.Sprintf("OutputPath error: %v", err))
	}

	// Merge task result back into the input document at ResultPath.
	resultPath := state.ResultPath
	if resultPath == "" {
		resultPath = "$" // AWS default: replace effective input with task output
	}
	merged, err := mergePath(input, filtered, resultPath)
	if err != nil {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName, fmt.Sprintf("ResultPath error: %v", err))
	}

	return &stateResult{output: merged, nextState: state.Next, end: state.End}, nil
}

// invokeWithRetry calls the scheduler, honouring the state's Retry config.
func (e *Executor) invokeWithRetry(
	ctx context.Context,
	workflowName, stateName string,
	state *config.State,
	payload []byte,
	logger *logrus.Entry,
) ([]byte, error) {
	attempt := 0

	for {
		output, err := e.scheduler.Invoke(ctx, state.Resource, payload)
		if err == nil {
			return output, nil
		}

		if len(state.Retry) == 0 {
			return nil, err
		}

		rc := matchRetry(state.Retry, err)
		if rc == nil {
			return nil, err
		}

		maxAttempts := rc.MaxAttempts
		if maxAttempts == 0 {
			maxAttempts = 3 // AWS default
		}

		if attempt >= maxAttempts {
			logger.WithError(err).Warnf("retry limit reached for state %s after %d attempts", stateName, attempt)
			return nil, err
		}

		interval := rc.IntervalSeconds
		if interval == 0 {
			interval = 1 // AWS default
		}
		backoffRate := rc.BackoffRate
		if backoffRate == 0 {
			backoffRate = 2.0 // AWS default
		}

		delay := float64(interval) * math.Pow(backoffRate, float64(attempt))
		if rc.Jitter {
			delay = delay * (0.5 + rand.Float64()*0.5)
		}

		attempt++
		logger.WithError(err).Warnf("retrying state %s (attempt %d/%d) after %.1fs", stateName, attempt, maxAttempts, delay)

		select {
		case <-ctx.Done():
			return nil, simlaerrors.NewWorkflowTimeoutError(workflowName, stateName)
		case <-time.After(time.Duration(delay * float64(time.Second))):
		}
	}
}

// matchRetry finds the first RetryConfig whose Errors list matches err.
// Returns nil if no config matches.
func matchRetry(retries []config.RetryConfig, err error) *config.RetryConfig {
	errName := classifyError(err)
	for i := range retries {
		for _, e := range retries[i].Errors {
			if e == ErrAll || e == errName {
				return &retries[i]
			}
		}
	}
	return nil
}

// tryCatch attempts to match err against the catch configs and returns the
// next state name, an updated output document, and whether a match was found.
func (e *Executor) tryCatch(catches []config.CatchConfig, err error, input []byte) (string, []byte, bool) {
	errName := classifyError(err)
	for _, c := range catches {
		for _, ce := range c.Errors {
			if ce == ErrAll || ce == errName {
				// Build an error object to put at ResultPath.
				errObj, _ := json.Marshal(map[string]string{
					"Error": errName,
					"Cause": err.Error(),
				})
				resultPath := c.ResultPath
				if resultPath == "" {
					resultPath = "$"
				}
				merged, mergeErr := mergePath(input, errObj, resultPath)
				if mergeErr != nil {
					merged = input
				}
				return c.Next, merged, true
			}
		}
	}
	return "", nil, false
}

// classifyError maps a Go error to an AWS-style error name.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "timed out") || strings.Contains(msg, "deadline exceeded"):
		return ErrTimeout
	case strings.Contains(msg, "not found"):
		return "States.ServiceNotFound"
	default:
		return ErrTaskFailed
	}
}

// ---------------------------------------------------------------------------
// Pass state
// ---------------------------------------------------------------------------

func (e *Executor) executePass(state *config.State, input []byte) (*stateResult, error) {
	var data []byte

	if state.Result != nil {
		// The static Result field replaces the effective input entirely.
		encoded, err := json.Marshal(state.Result)
		if err != nil {
			return nil, fmt.Errorf("Pass state: cannot marshal Result: %w", err)
		}
		data = encoded
	} else {
		// Apply InputPath first.
		effective, err := applyPath(input, state.InputPath)
		if err != nil {
			return nil, fmt.Errorf("Pass state InputPath error: %w", err)
		}
		data = effective
	}

	// Apply ResultPath to merge back.
	resultPath := state.ResultPath
	if resultPath == "" {
		resultPath = "$"
	}
	merged, err := mergePath(input, data, resultPath)
	if err != nil {
		return nil, fmt.Errorf("Pass state ResultPath error: %w", err)
	}

	// Apply OutputPath last.
	output, err := applyPath(merged, state.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("Pass state OutputPath error: %w", err)
	}

	return &stateResult{output: output, nextState: state.Next, end: state.End}, nil
}

// ---------------------------------------------------------------------------
// Choice state
// ---------------------------------------------------------------------------

func (e *Executor) executeChoice(
	workflowName, stateName string,
	state *config.State,
	input []byte,
) (*stateResult, error) {
	for _, rule := range state.Choices {
		eval := toChoiceRuleEval(&rule)
		matched, err := evaluateCondition(input, eval)
		if err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("error evaluating choice rule: %v", err))
		}
		if matched {
			return &stateResult{output: input, nextState: rule.Next}, nil
		}
	}

	if state.DefaultChoice != "" {
		return &stateResult{output: input, nextState: state.DefaultChoice}, nil
	}

	return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
		"no choice rule matched and no Default defined")
}

// toChoiceRuleEval converts a config.ChoiceRule to a ChoiceRuleEval, using
// pointer fields so the comparison helpers can distinguish "set to zero" from
// "not set at all".
func toChoiceRuleEval(r *config.ChoiceRule) ChoiceRuleEval {
	eval := ChoiceRuleEval{
		Variable:    r.Variable,
		Next:        r.Next,
		IsNull:      r.IsNull,
		IsPresent:   r.IsPresent,
		IsString:    r.IsString,
		IsNumeric:   r.IsNumeric,
		IsBoolean:   r.IsBoolean,
		IsTimestamp: r.IsTimestamp,
	}

	if r.StringEquals != "" {
		eval.StringEquals = &r.StringEquals
	}
	if r.StringLessThan != "" {
		eval.StringLessThan = &r.StringLessThan
	}
	if r.StringGreaterThan != "" {
		eval.StringGreaterThan = &r.StringGreaterThan
	}
	if r.StringLessThanEquals != "" {
		eval.StringLessThanEquals = &r.StringLessThanEquals
	}
	if r.StringGreaterThanEquals != "" {
		eval.StringGreaterThanEquals = &r.StringGreaterThanEquals
	}
	if r.StringMatches != "" {
		eval.StringMatches = &r.StringMatches
	}

	// Numeric comparisons — we use NumericEqualsPath as the sentinel for
	// "explicitly set", because config uses plain int (0 is ambiguous).
	// For float64 comparisons, cast from config's int fields.
	if r.NumericEqualsPath != "" {
		// path-based — not yet supported; treat as constant
		f := float64(r.NumericEquals)
		eval.NumericEquals = &f
	} else if r.NumericEquals != 0 {
		f := float64(r.NumericEquals)
		eval.NumericEquals = &f
	}
	if r.NumericLessThan != 0 {
		f := float64(r.NumericLessThan)
		eval.NumericLessThan = &f
	}
	if r.NumericGreaterThan != 0 {
		f := float64(r.NumericGreaterThan)
		eval.NumericGreaterThan = &f
	}
	if r.NumericLessThanEquals != 0 {
		f := float64(r.NumericLessThanEquals)
		eval.NumericLessThanEquals = &f
	}
	if r.NumericGreaterThanEquals != 0 {
		f := float64(r.NumericGreaterThanEquals)
		eval.NumericGreaterThanEquals = &f
	}

	if r.BooleanEqualsPath != "" || r.BooleanEquals {
		eval.BooleanEquals = &r.BooleanEquals
	}

	// Recursion for And / Or / Not.
	for _, sub := range r.And {
		sub := sub
		eval.And = append(eval.And, toChoiceRuleEval(&sub))
	}
	for _, sub := range r.Or {
		sub := sub
		eval.Or = append(eval.Or, toChoiceRuleEval(&sub))
	}
	if r.Not != nil {
		notEval := toChoiceRuleEval(r.Not)
		eval.Not = &notEval
	}

	return eval
}

// ---------------------------------------------------------------------------
// Parallel state
// ---------------------------------------------------------------------------

func (e *Executor) executeParallel(
	ctx context.Context,
	workflowName, stateName string,
	state *config.State,
	input []byte,
	logger *logrus.Entry,
) (*stateResult, error) {
	if len(state.Branches) == 0 {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName, "Parallel state has no branches")
	}

	type branchResult struct {
		index  int
		output []byte
		err    error
	}

	results := make([]branchResult, len(state.Branches))
	var wg sync.WaitGroup
	ch := make(chan branchResult, len(state.Branches))

	for i, branch := range state.Branches {
		branch := branch // capture loop variable
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			branchLogger := logger.WithField("branch", branch.Name)
			out, err := e.runMachine(ctx, &branch, input, branchLogger)
			ch <- branchResult{index: i, output: out, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for res := range ch {
		results[res.index] = res
	}

	// Collect errors — AWS behaviour: if any branch fails the whole state fails.
	for _, res := range results {
		if res.err != nil {
			// Try Catch blocks.
			if len(state.Catch) > 0 {
				next, catchOutput, matched := e.tryCatch(state.Catch, res.err, input)
				if matched {
					return &stateResult{output: catchOutput, nextState: next}, nil
				}
			}
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("branch %d failed: %v", res.index, res.err))
		}
	}

	// Combine branch outputs into a JSON array.
	outputs := make([]json.RawMessage, len(results))
	for i, res := range results {
		if res.output == nil {
			outputs[i] = json.RawMessage("null")
		} else {
			outputs[i] = res.output
		}
	}

	combined, err := json.Marshal(outputs)
	if err != nil {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
			fmt.Sprintf("cannot serialise parallel outputs: %v", err))
	}

	// Merge combined array back into the input document at ResultPath.
	resultPath := state.ResultPath
	if resultPath == "" {
		resultPath = "$"
	}
	merged, err := mergePath(input, combined, resultPath)
	if err != nil {
		return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
			fmt.Sprintf("ResultPath error: %v", err))
	}

	return &stateResult{output: merged, nextState: state.Next, end: state.End}, nil
}

// ---------------------------------------------------------------------------
// Wait state
// ---------------------------------------------------------------------------

func (e *Executor) executeWait(
	ctx context.Context,
	workflowName, stateName string,
	state *config.State,
	input []byte,
) (*stateResult, error) {
	var duration time.Duration

	switch {
	case state.Seconds > 0:
		duration = time.Duration(state.Seconds) * time.Second

	case state.SecondsPath != "":
		raw, err := applyPath(input, state.SecondsPath)
		if err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("SecondsPath error: %v", err))
		}
		var secs float64
		if err := json.Unmarshal(raw, &secs); err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("SecondsPath value is not a number: %v", err))
		}
		duration = time.Duration(secs) * time.Second

	case state.Timestamp != "":
		t, err := time.Parse(time.RFC3339, state.Timestamp)
		if err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("Timestamp parse error: %v", err))
		}
		duration = time.Until(t)

	case state.TimestampPath != "":
		raw, err := applyPath(input, state.TimestampPath)
		if err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("TimestampPath error: %v", err))
		}
		var ts string
		if err := json.Unmarshal(raw, &ts); err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("TimestampPath value is not a string: %v", err))
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, simlaerrors.NewWorkflowStateError(workflowName, stateName,
				fmt.Sprintf("TimestampPath timestamp parse error: %v", err))
		}
		duration = time.Until(t)

	default:
		// No wait configured — pass through immediately.
	}

	if duration > 0 {
		select {
		case <-ctx.Done():
			return nil, simlaerrors.NewWorkflowTimeoutError(workflowName, stateName)
		case <-time.After(duration):
		}
	}

	return &stateResult{output: input, nextState: state.Next, end: state.End}, nil
}
