package config

// TriggerType identifies the kind of event source for a Lambda trigger.
type TriggerType string

const (
	TriggerTypeSchedule        TriggerType = "schedule"
	TriggerTypeSQS             TriggerType = "sqs"
	TriggerTypeS3              TriggerType = "s3"
	TriggerTypeSNS             TriggerType = "sns"
	TriggerTypeDynamoDBStreams TriggerType = "dynamodb-stream"
)

// Trigger describes one event source attached to a service.
// Only the fields relevant to the chosen Type need to be populated.
type Trigger struct {
	// Type selects the trigger implementation.
	Type TriggerType `yaml:"type"`

	// --- Schedule / EventBridge ---
	// Expression is an AWS rate or cron expression, e.g. "rate(5 minutes)"
	// or "cron(0 12 * * ? *)".
	Expression string `yaml:"expression"`

	// --- SQS ---
	// QueueURL is the URL of an SQS-compatible queue endpoint,
	// e.g. "http://localhost:9324/queue/my-queue".
	QueueURL string `yaml:"queueUrl"`
	// BatchSize is the maximum number of messages per Lambda invocation (default 10).
	BatchSize int `yaml:"batchSize"`
	// PollingInterval is how often to poll the queue (default 1s).
	PollingInterval string `yaml:"pollingInterval"`

	// --- S3 ---
	// LocalPath is the host directory that maps to the S3 bucket.
	LocalPath string `yaml:"localPath"`
	// Bucket is the logical S3 bucket name used in the event payload.
	Bucket string `yaml:"bucket"`
	// Events is a list of S3 event names to react to, e.g. ["s3:ObjectCreated:*"].
	// An empty list defaults to all create and remove events.
	Events []string `yaml:"events"`

	// --- SNS ---
	// TopicARN is the ARN of the local SNS topic,
	// e.g. "arn:aws:sns:local:000000000000:my-topic".
	TopicARN string `yaml:"topicArn"`
	// SNSEndpointPort is the port on which simla listens for SNS Publish calls
	// for this topic (default 2772).
	SNSEndpointPort int `yaml:"snsEndpointPort"`

	// --- DynamoDB Streams ---
	// StreamARN is the ARN of the DynamoDB stream to consume.
	StreamARN string `yaml:"streamArn"`
	// DynamoDBEndpoint is the base URL of the local DynamoDB instance,
	// e.g. "http://localhost:8000".
	DynamoDBEndpoint string `yaml:"dynamodbEndpoint"`
	// StartingPosition is TRIM_HORIZON or LATEST (default LATEST).
	StartingPosition string `yaml:"startingPosition"`
}

type Service struct {
	Runtime      string            `yaml:"runtime"`
	Image        string            `yaml:"image"`
	Architecture string            `yaml:"architecture"`
	CodePath     string            `yaml:"codePath"`
	Cmd          []string          `yaml:"cmd"`
	Entrypoint   []string          `yaml:"entrypoint"`
	Environment  map[string]string `yaml:"environment"`
	// EnvFile is a path to a .env file whose variables are merged on top of
	// Environment. Values in both sources may use ${VAR} interpolation against
	// the host environment.
	EnvFile  string    `yaml:"envFile"`
	Triggers []Trigger `yaml:"triggers"`
}

type Route struct {
	Path    string `yaml:"path"`
	Service string `yaml:"service"`
	Method  string `yaml:"method"`
}

// CORSConfig controls cross-origin resource sharing headers added by the
// gateway. Set Enabled: true to activate; all other fields have sensible
// defaults.
type CORSConfig struct {
	// Enabled turns CORS headers on. When false the remaining fields are ignored.
	Enabled bool `yaml:"enabled"`
	// AllowOrigins is the value of Access-Control-Allow-Origin.
	// Defaults to "*".
	AllowOrigins string `yaml:"allowOrigins"`
	// AllowMethods is the value of Access-Control-Allow-Methods.
	// Defaults to "GET,POST,PUT,PATCH,DELETE,OPTIONS".
	AllowMethods string `yaml:"allowMethods"`
	// AllowHeaders is the value of Access-Control-Allow-Headers.
	// Defaults to "Content-Type,Authorization,X-Request-ID".
	AllowHeaders string `yaml:"allowHeaders"`
	// AllowCredentials sets Access-Control-Allow-Credentials: true when enabled.
	AllowCredentials bool `yaml:"allowCredentials"`
	// MaxAge is the Access-Control-Max-Age value in seconds (preflight cache).
	// Defaults to 86400 (24 h).
	MaxAge int `yaml:"maxAge"`
}

type APIGateway struct {
	Port   string     `yaml:"port"`
	Routes []Route    `yaml:"routes"`
	Stage  string     `yaml:"stage"`
	CORS   CORSConfig `yaml:"cors"`
}

type Config struct {
	APIGateway APIGateway              `yaml:"apiGateway"`
	Services   map[string]Service      `yaml:"services"`
	Workflows  map[string]StateMachine `yaml:"workflows"`
	Host       string                  `yaml:"-"`
}
