package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeConfig() *Config {
	return &Config{
		Host: "127.0.0.1",
		APIGateway: APIGateway{
			Port:  "8080",
			Stage: "v1",
			Routes: []Route{
				{Path: "payments", Method: "GET", Service: "payments"},
			},
		},
		Services: map[string]Service{
			"payments": {
				Runtime:      "go",
				Architecture: "amd64",
				CodePath:     "./bin",
				Cmd:          []string{"main"},
				Environment:  map[string]string{"ENV": "test"},
			},
			"orders": {
				Image:    "public.ecr.aws/lambda/python:3.13",
				CodePath: "./python",
				Cmd:      []string{"main.handler"},
				Triggers: []Trigger{
					{Type: TriggerTypeSQS, QueueURL: "http://localhost:9324/queue/orders"},
				},
			},
		},
	}
}

// ── GetService ────────────────────────────────────────────────────────────────

func TestGetService_Found(t *testing.T) {
	cfg := makeConfig()
	svc, ok := cfg.GetService(context.Background(), "payments")
	require.True(t, ok)
	require.NotNil(t, svc)
	assert.Equal(t, "go", svc.Runtime)
	assert.Equal(t, "amd64", svc.Architecture)
	assert.Equal(t, []string{"main"}, svc.Cmd)
}

func TestGetService_NotFound(t *testing.T) {
	cfg := makeConfig()
	svc, ok := cfg.GetService(context.Background(), "nonexistent")
	assert.False(t, ok)
	assert.Nil(t, svc)
}

func TestGetService_EmptyName(t *testing.T) {
	cfg := makeConfig()
	svc, ok := cfg.GetService(context.Background(), "")
	assert.False(t, ok)
	assert.Nil(t, svc)
}

func TestGetService_ReturnsPointerToIndependentCopy(t *testing.T) {
	// GetService returns &service (a copy from the map value), so mutations
	// on the returned pointer must not affect the map entry.
	cfg := makeConfig()
	svc, ok := cfg.GetService(context.Background(), "payments")
	require.True(t, ok)
	svc.Runtime = "python"

	original, _ := cfg.GetService(context.Background(), "payments")
	assert.Equal(t, "go", original.Runtime, "map entry should be unchanged")
}

func TestGetService_WithTriggers(t *testing.T) {
	cfg := makeConfig()
	svc, ok := cfg.GetService(context.Background(), "orders")
	require.True(t, ok)
	require.Len(t, svc.Triggers, 1)
	assert.Equal(t, TriggerTypeSQS, svc.Triggers[0].Type)
	assert.Equal(t, "http://localhost:9324/queue/orders", svc.Triggers[0].QueueURL)
}

// ── TriggerType constants ─────────────────────────────────────────────────────

func TestTriggerTypeConstants(t *testing.T) {
	assert.Equal(t, TriggerType("schedule"), TriggerTypeSchedule)
	assert.Equal(t, TriggerType("sqs"), TriggerTypeSQS)
	assert.Equal(t, TriggerType("s3"), TriggerTypeS3)
	assert.Equal(t, TriggerType("sns"), TriggerTypeSNS)
	assert.Equal(t, TriggerType("dynamodb-stream"), TriggerTypeDynamoDBStreams)
}

// ── Trigger struct fields ─────────────────────────────────────────────────────

func TestTrigger_ScheduleFields(t *testing.T) {
	trig := Trigger{
		Type:       TriggerTypeSchedule,
		Expression: "rate(5 minutes)",
	}
	assert.Equal(t, TriggerTypeSchedule, trig.Type)
	assert.Equal(t, "rate(5 minutes)", trig.Expression)
}

func TestTrigger_SQSFields(t *testing.T) {
	trig := Trigger{
		Type:            TriggerTypeSQS,
		QueueURL:        "http://localhost:9324/queue/my-queue",
		BatchSize:       5,
		PollingInterval: "2s",
	}
	assert.Equal(t, 5, trig.BatchSize)
	assert.Equal(t, "2s", trig.PollingInterval)
}

func TestTrigger_S3Fields(t *testing.T) {
	trig := Trigger{
		Type:      TriggerTypeS3,
		LocalPath: "./data/uploads",
		Bucket:    "my-bucket",
		Events:    []string{"s3:ObjectCreated:*"},
	}
	assert.Equal(t, "./data/uploads", trig.LocalPath)
	assert.Equal(t, "my-bucket", trig.Bucket)
	assert.Equal(t, []string{"s3:ObjectCreated:*"}, trig.Events)
}

func TestTrigger_SNSFields(t *testing.T) {
	trig := Trigger{
		Type:            TriggerTypeSNS,
		TopicARN:        "arn:aws:sns:local:000000000000:topic",
		SNSEndpointPort: 2772,
	}
	assert.Equal(t, "arn:aws:sns:local:000000000000:topic", trig.TopicARN)
	assert.Equal(t, 2772, trig.SNSEndpointPort)
}

func TestTrigger_DynamoDBFields(t *testing.T) {
	trig := Trigger{
		Type:             TriggerTypeDynamoDBStreams,
		StreamARN:        "arn:aws:dynamodb:local:000:table/t/stream/2024",
		DynamoDBEndpoint: "http://localhost:8000",
		StartingPosition: "TRIM_HORIZON",
	}
	assert.Equal(t, "arn:aws:dynamodb:local:000:table/t/stream/2024", trig.StreamARN)
	assert.Equal(t, "http://localhost:8000", trig.DynamoDBEndpoint)
	assert.Equal(t, "TRIM_HORIZON", trig.StartingPosition)
}

// ── Empty/nil config edge cases ───────────────────────────────────────────────

func TestGetService_NilServicesMap(t *testing.T) {
	cfg := &Config{Services: nil}
	svc, ok := cfg.GetService(context.Background(), "anything")
	assert.False(t, ok)
	assert.Nil(t, svc)
}

func TestGetService_EmptyServicesMap(t *testing.T) {
	cfg := &Config{Services: map[string]Service{}}
	svc, ok := cfg.GetService(context.Background(), "anything")
	assert.False(t, ok)
	assert.Nil(t, svc)
}
