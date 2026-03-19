package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// helpers

func newLogger() *logrus.Entry {
	l := logrus.New()
	l.SetOutput(os.Stdout)
	l.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(l)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// buildCfg builds a minimal Config with a single workflow and the named services.
func buildCfg(sm config.StateMachine) *config.Config {
	return &config.Config{
		Services: map[string]config.Service{
			"svc-a": {Runtime: "go", CodePath: ".", Cmd: []string{"main"}},
			"svc-b": {Runtime: "go", CodePath: ".", Cmd: []string{"main"}},
		},
		Workflows: map[string]config.StateMachine{
			sm.Name: sm,
		},
	}
}

// ---------------------------------------------------------------------------
// Execute – workflow not found
// ---------------------------------------------------------------------------

func TestExecute_WorkflowNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	cfg := &config.Config{Workflows: map[string]config.StateMachine{}}

	ex := NewExecutor(cfg, sched, newLogger())
	_, err := ex.Execute(context.Background(), "missing", []byte("{}"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

// ---------------------------------------------------------------------------
// Pass state
// ---------------------------------------------------------------------------

func TestExecute_PassState_BasicPassthrough(t *testing.T) {
	sm := config.StateMachine{
		Name:    "pass-test",
		StartAt: "p1",
		States: map[string]config.State{
			"p1": {Type: "Pass", End: true},
		},
	}
	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	out, err := ex.Execute(context.Background(), "pass-test", mustJSON(map[string]string{"hello": "world"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"hello":"world"}`, string(out))
}

func TestExecute_PassState_StaticResult(t *testing.T) {
	sm := config.StateMachine{
		Name:    "pass-result",
		StartAt: "p1",
		States: map[string]config.State{
			"p1": {Type: "Pass", Result: map[string]string{"injected": "value"}, End: true},
		},
	}
	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	out, err := ex.Execute(context.Background(), "pass-result", mustJSON(map[string]string{"original": "data"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"injected":"value"}`, string(out))
}

func TestExecute_PassState_ResultPath(t *testing.T) {
	sm := config.StateMachine{
		Name:    "pass-resultpath",
		StartAt: "p1",
		States: map[string]config.State{
			"p1": {
				Type:       "Pass",
				Result:     map[string]string{"status": "ok"},
				ResultPath: "$.taskResult",
				End:        true,
			},
		},
	}
	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	out, err := ex.Execute(context.Background(), "pass-resultpath", mustJSON(map[string]string{"input": "data"}))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "data", got["input"])
	assert.Equal(t, map[string]any{"status": "ok"}, got["taskResult"])
}

// ---------------------------------------------------------------------------
// Succeed & Fail states
// ---------------------------------------------------------------------------

func TestExecute_SucceedState(t *testing.T) {
	sm := config.StateMachine{
		Name:    "succeed-test",
		StartAt: "done",
		States: map[string]config.State{
			"done": {Type: "Succeed"},
		},
	}
	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	input := mustJSON(map[string]int{"count": 42})
	out, err := ex.Execute(context.Background(), "succeed-test", input)
	require.NoError(t, err)
	assert.JSONEq(t, string(input), string(out))
}

func TestExecute_FailState(t *testing.T) {
	sm := config.StateMachine{
		Name:    "fail-test",
		StartAt: "boom",
		States: map[string]config.State{
			"boom": {Type: "Fail", Error: "MyError", Cause: "intentional failure"},
		},
	}
	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	_, err := ex.Execute(context.Background(), "fail-test", []byte("{}"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional failure")
}

// ---------------------------------------------------------------------------
// Task state
// ---------------------------------------------------------------------------

func TestExecute_TaskState_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	output := mustJSON(map[string]string{"result": "done"})
	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).Return(output, nil)

	sm := config.StateMachine{
		Name:    "task-test",
		StartAt: "step1",
		States: map[string]config.State{
			"step1": {Type: "Task", Resource: "svc-a", End: true},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "task-test", []byte("{}"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"result":"done"}`, string(out))
}

func TestExecute_TaskState_InputPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	// Verify the scheduler receives only the nested payload.
	sched.EXPECT().
		Invoke(gomock.Any(), "svc-a", mustJSON(map[string]string{"name": "Alice"})).
		Return(mustJSON(map[string]string{"greeted": "Alice"}), nil)

	sm := config.StateMachine{
		Name:    "input-path-test",
		StartAt: "greet",
		States: map[string]config.State{
			"greet": {Type: "Task", Resource: "svc-a", InputPath: "$.user", End: true},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	input := mustJSON(map[string]any{"user": map[string]string{"name": "Alice"}, "extra": "ignored"})
	out, err := ex.Execute(context.Background(), "input-path-test", input)
	require.NoError(t, err)
	assert.JSONEq(t, `{"greeted":"Alice"}`, string(out))
}

func TestExecute_TaskState_ResultPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(mustJSON(map[string]string{"status": "ok"}), nil)

	sm := config.StateMachine{
		Name:    "result-path-test",
		StartAt: "step1",
		States: map[string]config.State{
			"step1": {
				Type:       "Task",
				Resource:   "svc-a",
				ResultPath: "$.taskOutput",
				End:        true,
			},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "result-path-test", mustJSON(map[string]string{"original": "data"}))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "data", got["original"])
	assert.Equal(t, map[string]any{"status": "ok"}, got["taskOutput"])
}

func TestExecute_TaskState_Catch(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(nil, errors.New("service unavailable"))

	sm := config.StateMachine{
		Name:    "catch-test",
		StartAt: "risky",
		States: map[string]config.State{
			"risky": {
				Type:     "Task",
				Resource: "svc-a",
				Catch: []config.CatchConfig{
					{Errors: []string{"States.ALL"}, Next: "fallback", ResultPath: "$.error"},
				},
				Next: "shouldNotReach",
			},
			"fallback":       {Type: "Succeed"},
			"shouldNotReach": {Type: "Fail", Cause: "reached wrong state"},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "catch-test", []byte("{}"))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.NotNil(t, got["error"])
}

func TestExecute_TaskState_Retry_ThenSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	gomock.InOrder(
		sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).Return(nil, errors.New("transient")),
		sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).Return(mustJSON(map[string]string{"ok": "true"}), nil),
	)

	sm := config.StateMachine{
		Name:    "retry-test",
		StartAt: "flaky",
		States: map[string]config.State{
			"flaky": {
				Type:     "Task",
				Resource: "svc-a",
				Retry: []config.RetryConfig{
					{Errors: []string{"States.ALL"}, MaxAttempts: 2, IntervalSeconds: 0, BackoffRate: 1},
				},
				End: true,
			},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "retry-test", []byte("{}"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":"true"}`, string(out))
}

func TestExecute_TaskState_Retry_Exhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(nil, errors.New("always fails")).Times(3)

	sm := config.StateMachine{
		Name:    "retry-exhaust",
		StartAt: "bad",
		States: map[string]config.State{
			"bad": {
				Type:     "Task",
				Resource: "svc-a",
				Retry: []config.RetryConfig{
					{Errors: []string{"States.ALL"}, MaxAttempts: 2, IntervalSeconds: 0, BackoffRate: 1},
				},
				End: true,
			},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	_, err := ex.Execute(context.Background(), "retry-exhaust", []byte("{}"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "always fails")
}

// ---------------------------------------------------------------------------
// Choice state
// ---------------------------------------------------------------------------

func TestExecute_ChoiceState_StringEquals(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(mustJSON(map[string]string{"branch": "premium"}), nil)

	sm := config.StateMachine{
		Name:    "choice-test",
		StartAt: "decide",
		States: map[string]config.State{
			"decide": {
				Type: "Choice",
				Choices: []config.ChoiceRule{
					{Variable: "$.tier", StringEquals: "premium", Next: "premiumFlow"},
					{Variable: "$.tier", StringEquals: "basic", Next: "basicFlow"},
				},
				DefaultChoice: "basicFlow",
			},
			"premiumFlow": {Type: "Task", Resource: "svc-a", End: true},
			"basicFlow":   {Type: "Succeed"},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "choice-test", mustJSON(map[string]string{"tier": "premium"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"branch":"premium"}`, string(out))
}

func TestExecute_ChoiceState_Default(t *testing.T) {
	sm := config.StateMachine{
		Name:    "choice-default",
		StartAt: "decide",
		States: map[string]config.State{
			"decide": {
				Type: "Choice",
				Choices: []config.ChoiceRule{
					{Variable: "$.tier", StringEquals: "premium", Next: "premiumFlow"},
				},
				DefaultChoice: "fallback",
			},
			"premiumFlow": {Type: "Succeed"},
			"fallback":    {Type: "Succeed"},
		},
	}

	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	out, err := ex.Execute(context.Background(), "choice-default", mustJSON(map[string]string{"tier": "basic"}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"tier":"basic"}`, string(out))
}

func TestExecute_ChoiceState_NoMatch_NoDefault(t *testing.T) {
	sm := config.StateMachine{
		Name:    "choice-nomatch",
		StartAt: "decide",
		States: map[string]config.State{
			"decide": {
				Type: "Choice",
				Choices: []config.ChoiceRule{
					{Variable: "$.tier", StringEquals: "premium", Next: "premiumFlow"},
				},
			},
			"premiumFlow": {Type: "Succeed"},
		},
	}

	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	_, err := ex.Execute(context.Background(), "choice-nomatch", mustJSON(map[string]string{"tier": "unknown"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choice rule matched")
}

// ---------------------------------------------------------------------------
// Wait state
// ---------------------------------------------------------------------------

func TestExecute_WaitState_Seconds(t *testing.T) {
	sm := config.StateMachine{
		Name:    "wait-test",
		StartAt: "pause",
		States: map[string]config.State{
			"pause": {Type: "Wait", Seconds: 0, Next: "done"}, // 0s so test is instant
			"done":  {Type: "Succeed"},
		},
	}

	ex := NewExecutor(buildCfg(sm), nil, newLogger())
	input := mustJSON(map[string]string{"data": "value"})
	out, err := ex.Execute(context.Background(), "wait-test", input)
	require.NoError(t, err)
	assert.JSONEq(t, string(input), string(out))
}

// ---------------------------------------------------------------------------
// Parallel state
// ---------------------------------------------------------------------------

func TestExecute_ParallelState_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(mustJSON(map[string]string{"branch": "a"}), nil)
	sched.EXPECT().Invoke(gomock.Any(), "svc-b", gomock.Any()).
		Return(mustJSON(map[string]string{"branch": "b"}), nil)

	sm := config.StateMachine{
		Name:    "parallel-test",
		StartAt: "both",
		States: map[string]config.State{
			"both": {
				Type: "Parallel",
				Branches: []config.StateMachine{
					{
						Name:    "branchA",
						StartAt: "a",
						States: map[string]config.State{
							"a": {Type: "Task", Resource: "svc-a", End: true},
						},
					},
					{
						Name:    "branchB",
						StartAt: "b",
						States: map[string]config.State{
							"b": {Type: "Task", Resource: "svc-b", End: true},
						},
					},
				},
				ResultPath: "$.parallel",
				End:        true,
			},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "parallel-test", []byte("{}"))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	parallelResults, ok := got["parallel"].([]any)
	require.True(t, ok, "$.parallel should be an array")
	assert.Len(t, parallelResults, 2)
}

func TestExecute_ParallelState_BranchFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
		Return(nil, errors.New("branch A failed")).AnyTimes()
	sched.EXPECT().Invoke(gomock.Any(), "svc-b", gomock.Any()).
		Return(mustJSON(map[string]string{"ok": "true"}), nil).AnyTimes()

	sm := config.StateMachine{
		Name:    "parallel-fail",
		StartAt: "both",
		States: map[string]config.State{
			"both": {
				Type: "Parallel",
				Branches: []config.StateMachine{
					{
						Name:    "branchA",
						StartAt: "a",
						States:  map[string]config.State{"a": {Type: "Task", Resource: "svc-a", End: true}},
					},
					{
						Name:    "branchB",
						StartAt: "b",
						States:  map[string]config.State{"b": {Type: "Task", Resource: "svc-b", End: true}},
					},
				},
				End: true,
			},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	_, err := ex.Execute(context.Background(), "parallel-fail", []byte("{}"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch")
}

// ---------------------------------------------------------------------------
// Multi-step linear workflow
// ---------------------------------------------------------------------------

func TestExecute_LinearMultiStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	gomock.InOrder(
		sched.EXPECT().Invoke(gomock.Any(), "svc-a", gomock.Any()).
			Return(mustJSON(map[string]string{"step": "one"}), nil),
		sched.EXPECT().Invoke(gomock.Any(), "svc-b", gomock.Any()).
			Return(mustJSON(map[string]string{"step": "two"}), nil),
	)

	sm := config.StateMachine{
		Name:    "linear",
		StartAt: "step1",
		States: map[string]config.State{
			"step1": {Type: "Task", Resource: "svc-a", Next: "step2"},
			"step2": {Type: "Task", Resource: "svc-b", End: true},
		},
	}

	ex := NewExecutor(buildCfg(sm), sched, newLogger())
	out, err := ex.Execute(context.Background(), "linear", []byte("{}"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"step":"two"}`, string(out))
}
