package config

import (
	"context"
)

const (
	StateTypeTask     StateType = "Task"
	StateTypePass     StateType = "Pass"
	StateTypeChoice   StateType = "Choice"
	StateTypeParallel StateType = "Parallel"
	StateTypeWait     StateType = "Wait"
	StateTypeSucceed  StateType = "Succeed"
	StateTypeFail     StateType = "Fail"
)

type StateType string

type StateMachine struct {
	Name    string           `yaml:"name"`
	Comment string           `yaml:"comment"`
	StartAt string           `yaml:"startAt"`
	States  map[string]State `yaml:"states"`
	Version string           `yaml:"version"`
}

type State struct {
	Type string `yaml:"Type"`

	Next    string `yaml:"Next"`
	End     bool   `yaml:"End"`
	Comment string `yaml:"comment"`

	Resource string `yaml:"resource"`

	InputPath  string `yaml:"inputPath"`
	OutputPath string `yaml:"outputPath"`
	ResultPath string `yaml:"resultPath"`
	Result     any    `yaml:"result"`

	TimeoutSeconds   int `yaml:"timeoutSeconds"`
	HeartbeatSeconds int `yaml:"heartbeatSeconds"`

	Retry []RetryConfig `yaml:"retry"`
	Catch []CatchConfig `yaml:"catch"`

	Branches []StateMachine `yaml:"branches"`

	Choices       []ChoiceRule `yaml:"choices"`
	DefaultChoice string       `yaml:"default"`

	Seconds       int    `yaml:"seconds"`
	SecondsPath   string `yaml:"secondsPath"`
	Timestamp     string `yaml:"timestamp"`
	TimestampPath string `yaml:"timestampPath"`

	Error     string `yaml:"error"`
	Cause     string `yaml:"cause"`
	CausePath string `yaml:"causePath"`
}

type RetryConfig struct {
	Errors          []string `yaml:"errors"`
	IntervalSeconds int      `yaml:"intervalSeconds"`
	MaxAttempts     int      `yaml:"maxAttempts"`
	BackoffRate     float64  `yaml:"backoffRate"`
	Jitter          bool     `yaml:"jitter"`
}

type CatchConfig struct {
	Errors     []string `yaml:"errors"`
	Next       string   `yaml:"next"`
	ResultPath string   `yaml:"resultPath"`
}

type ChoiceRule struct {
	Variable                 string       `yaml:"variable"`
	StringEquals             string       `yaml:"stringEquals"`
	StringLessThan           string       `yaml:"stringLessThan"`
	StringGreaterThan        string       `yaml:"stringGreaterThan"`
	StringLessThanEquals     string       `yaml:"stringLessThanEquals"`
	StringGreaterThanEquals  string       `yaml:"stringGreaterThanEquals"`
	StringMatches            string       `yaml:"stringMatches"`
	NumericEquals            int          `yaml:"numericEquals"`
	NumericLessThan          int          `yaml:"numericLessThan"`
	NumericGreaterThan       int          `yaml:"numericGreaterThan"`
	NumericLessThanEquals    int          `yaml:"numericLessThanEquals"`
	NumericGreaterThanEquals int          `yaml:"numericGreaterThanEquals"`
	NumericEqualsPath        string       `yaml:"numericEqualsPath"`
	BooleanEquals            bool         `yaml:"booleanEquals"`
	BooleanEqualsPath        string       `yaml:"booleanEqualsPath"`
	IsNull                   bool         `yaml:"isNull"`
	IsPresent                bool         `yaml:"isPresent"`
	IsString                 bool         `yaml:"isString"`
	IsNumeric                bool         `yaml:"isNumeric"`
	IsBoolean                bool         `yaml:"isBoolean"`
	IsTimestamp              bool         `yaml:"isTimestamp"`
	And                      []ChoiceRule `yaml:"and"`
	Or                       []ChoiceRule `yaml:"or"`
	Not                      *ChoiceRule  `yaml:"not"`
	Next                     string       `yaml:"next"`
}

func (c *Config) GetWorkflow(ctx context.Context, workflowName string) (*StateMachine, bool) {
	if workflow, exists := c.Workflows[workflowName]; exists {
		return &workflow, true
	}
	return nil, false
}
