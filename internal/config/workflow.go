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
	Name    string           `yaml:"name"    mapstructure:"name"`
	Comment string           `yaml:"comment" mapstructure:"comment"`
	StartAt string           `yaml:"startAt" mapstructure:"startat"`
	States  map[string]State `yaml:"states"  mapstructure:"states"`
	Version string           `yaml:"version" mapstructure:"version"`
}

type State struct {
	Type string `yaml:"type" mapstructure:"type"`

	Next    string `yaml:"next"    mapstructure:"next"`
	End     bool   `yaml:"end"     mapstructure:"end"`
	Comment string `yaml:"comment" mapstructure:"comment"`

	Resource string `yaml:"resource" mapstructure:"resource"`

	InputPath  string `yaml:"inputPath"  mapstructure:"inputpath"`
	OutputPath string `yaml:"outputPath" mapstructure:"outputpath"`
	ResultPath string `yaml:"resultPath" mapstructure:"resultpath"`
	Result     any    `yaml:"result"     mapstructure:"result"`

	TimeoutSeconds   int `yaml:"timeoutSeconds"   mapstructure:"timeoutseconds"`
	HeartbeatSeconds int `yaml:"heartbeatSeconds" mapstructure:"heartbeatseconds"`

	Retry []RetryConfig `yaml:"retry" mapstructure:"retry"`
	Catch []CatchConfig `yaml:"catch" mapstructure:"catch"`

	Branches []StateMachine `yaml:"branches" mapstructure:"branches"`

	Choices       []ChoiceRule `yaml:"choices" mapstructure:"choices"`
	DefaultChoice string       `yaml:"default" mapstructure:"default"`

	Seconds       int    `yaml:"seconds"       mapstructure:"seconds"`
	SecondsPath   string `yaml:"secondsPath"   mapstructure:"secondspath"`
	Timestamp     string `yaml:"timestamp"     mapstructure:"timestamp"`
	TimestampPath string `yaml:"timestampPath" mapstructure:"timestamppath"`

	Error     string `yaml:"error"      mapstructure:"error"`
	Cause     string `yaml:"cause"      mapstructure:"cause"`
	CausePath string `yaml:"causePath"  mapstructure:"causepath"`
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
	Variable                 string       `yaml:"variable"                  mapstructure:"variable"`
	StringEquals             string       `yaml:"stringEquals"              mapstructure:"stringequals"`
	StringLessThan           string       `yaml:"stringLessThan"            mapstructure:"stringlessthan"`
	StringGreaterThan        string       `yaml:"stringGreaterThan"         mapstructure:"stringgreaterthan"`
	StringLessThanEquals     string       `yaml:"stringLessThanEquals"      mapstructure:"stringlessthanequals"`
	StringGreaterThanEquals  string       `yaml:"stringGreaterThanEquals"   mapstructure:"stringgreaterthanequals"`
	StringMatches            string       `yaml:"stringMatches"             mapstructure:"stringmatches"`
	NumericEquals            int          `yaml:"numericEquals"             mapstructure:"numericequals"`
	NumericLessThan          int          `yaml:"numericLessThan"           mapstructure:"numericlessthan"`
	NumericGreaterThan       int          `yaml:"numericGreaterThan"        mapstructure:"numericgreaterthan"`
	NumericLessThanEquals    int          `yaml:"numericLessThanEquals"     mapstructure:"numericlessthanequals"`
	NumericGreaterThanEquals int          `yaml:"numericGreaterThanEquals"  mapstructure:"numericgreaterthanequals"`
	NumericEqualsPath        string       `yaml:"numericEqualsPath"         mapstructure:"numericqualspath"`
	BooleanEquals            bool         `yaml:"booleanEquals"             mapstructure:"booleanequals"`
	BooleanEqualsPath        string       `yaml:"booleanEqualsPath"         mapstructure:"booleanqualspath"`
	IsNull                   bool         `yaml:"isNull"      mapstructure:"isnull"`
	IsPresent                bool         `yaml:"isPresent"   mapstructure:"ispresent"`
	IsString                 bool         `yaml:"isString"    mapstructure:"isstring"`
	IsNumeric                bool         `yaml:"isNumeric"   mapstructure:"isnumeric"`
	IsBoolean                bool         `yaml:"isBoolean"   mapstructure:"isboolean"`
	IsTimestamp              bool         `yaml:"isTimestamp" mapstructure:"istimestamp"`
	And                      []ChoiceRule `yaml:"and"  mapstructure:"and"`
	Or                       []ChoiceRule `yaml:"or"   mapstructure:"or"`
	Not                      *ChoiceRule  `yaml:"not"  mapstructure:"not"`
	Next                     string       `yaml:"next" mapstructure:"next"`
}

func (c *Config) GetWorkflow(ctx context.Context, workflowName string) (*StateMachine, bool) {
	if workflow, exists := c.Workflows[workflowName]; exists {
		return &workflow, true
	}
	return nil, false
}
