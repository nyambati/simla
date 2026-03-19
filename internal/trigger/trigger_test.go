package trigger

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newBase builds a base with a mock scheduler and discarded logs.
func newBase(t *testing.T, serviceName string, sched *mocks.MockSchedulerInterface) base {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	return base{
		serviceName: serviceName,
		scheduler:   sched,
		logger:      logger.WithField("component", "trigger"),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Factory — trigger.New
// ─────────────────────────────────────────────────────────────────────────────

func TestNew_UnknownType_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	_, err := New(config.Trigger{Type: "unknown"}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	var unknown *UnknownTriggerTypeError
	assert.True(t, errors.As(err, &unknown))
	assert.Equal(t, "unknown", unknown.Type)
}

func TestNew_Schedule_MissingExpression_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeSchedule}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expression is required")
}

func TestNew_SQS_MissingQueueURL_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeSQS}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queueUrl is required")
}

func TestNew_S3_MissingLocalPath_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeS3, Bucket: "b"}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "localPath is required")
}

func TestNew_S3_MissingBucket_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeS3, LocalPath: t.TempDir()}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

func TestNew_SNS_MissingTopicARN_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeSNS}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "topicArn is required")
}

func TestNew_DynamoDB_MissingStreamARN_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeDynamoDBStreams, DynamoDBEndpoint: "http://localhost:8000"}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "streamArn is required")
}

func TestNew_DynamoDB_MissingEndpoint_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	_, err := New(config.Trigger{Type: config.TriggerTypeDynamoDBStreams, StreamARN: "arn:..."}, "svc", sched, logrus.NewEntry(logrus.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dynamodbEndpoint is required")
}

// ─────────────────────────────────────────────────────────────────────────────
// Schedule trigger
// ─────────────────────────────────────────────────────────────────────────────

func TestParseExpression_Rate_Minutes(t *testing.T) {
	d, err := parseExpression("rate(1 minute)")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, d)
}

func TestParseExpression_Rate_PluralMinutes(t *testing.T) {
	d, err := parseExpression("rate(5 minutes)")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, d)
}

func TestParseExpression_Rate_Hours(t *testing.T) {
	d, err := parseExpression("rate(2 hours)")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Hour, d)
}

func TestParseExpression_Rate_Days(t *testing.T) {
	d, err := parseExpression("rate(3 days)")
	require.NoError(t, err)
	assert.Equal(t, 3*24*time.Hour, d)
}

func TestParseExpression_Rate_ZeroValue_ReturnsError(t *testing.T) {
	_, err := parseExpression("rate(0 minutes)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive integer")
}

func TestParseExpression_Rate_NegativeValue_ReturnsError(t *testing.T) {
	_, err := parseExpression("rate(-1 minutes)")
	require.Error(t, err)
}

func TestParseExpression_Rate_UnknownUnit_ReturnsError(t *testing.T) {
	_, err := parseExpression("rate(5 seconds)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported rate unit")
}

func TestParseExpression_Rate_MissingUnit_ReturnsError(t *testing.T) {
	_, err := parseExpression("rate(5)")
	require.Error(t, err)
}

func TestParseExpression_Cron_MinuteField(t *testing.T) {
	d, err := parseExpression("cron(30 * * * ? *)")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, d)
}

func TestParseExpression_Cron_HourField(t *testing.T) {
	d, err := parseExpression("cron(* 12 * * ? *)")
	require.NoError(t, err)
	assert.Equal(t, time.Hour, d)
}

func TestParseExpression_Cron_DayField(t *testing.T) {
	d, err := parseExpression("cron(* * 1 * ? *)")
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d)
}

func TestParseExpression_Cron_AllWildcards_Returns24h(t *testing.T) {
	d, err := parseExpression("cron(* * * * ? *)")
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d)
}

func TestParseExpression_Cron_WrongFieldCount_ReturnsError(t *testing.T) {
	_, err := parseExpression("cron(* * *)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "6 fields")
}

func TestParseExpression_UnsupportedPrefix_ReturnsError(t *testing.T) {
	_, err := parseExpression("every(5 minutes)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported expression")
}

func TestParseExpression_Empty_ReturnsError(t *testing.T) {
	_, err := parseExpression("")
	require.Error(t, err)
}

func TestBuildSchedulePayload_Fields(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	data, err := buildSchedulePayload("rate(5 minutes)", now)
	require.NoError(t, err)

	var evt events.CloudWatchEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	assert.Equal(t, "0", evt.Version)
	assert.Equal(t, "Scheduled Event", evt.DetailType)
	assert.Equal(t, "aws.events", evt.Source)
	assert.Equal(t, "012345678901", evt.AccountID)
	assert.NotEmpty(t, evt.ID)
	assert.Contains(t, evt.Resources[0], "rate(5 minutes)")
}

func TestScheduleTrigger_Start_InvokesAndStopsOnCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	invoked := make(chan struct{}, 5)
	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ []byte) ([]byte, error) {
			invoked <- struct{}{}
			return []byte(`"ok"`), nil
		}).AnyTimes()

	src := &scheduleTrigger{
		base:       newBase(t, "svc", sched),
		interval:   50 * time.Millisecond,
		expression: "rate(1 minute)",
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- src.Start(ctx) }()

	// Wait for at least two invocations then cancel.
	<-invoked
	<-invoked
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SQS trigger
// ─────────────────────────────────────────────────────────────────────────────

func TestNewSQS_DefaultBatchSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newSQS(config.Trigger{QueueURL: "http://localhost/queue/test"}, b)
	require.NoError(t, err)
	assert.Equal(t, sqsDefaultBatchSize, src.(*sqsTrigger).batchSize)
}

func TestNewSQS_BatchSizeCappedAt10(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newSQS(config.Trigger{QueueURL: "http://localhost/queue/test", BatchSize: 99}, b)
	require.NoError(t, err)
	assert.Equal(t, 10, src.(*sqsTrigger).batchSize)
}

func TestNewSQS_InvalidPollingInterval_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	_, err := newSQS(config.Trigger{QueueURL: "http://localhost/q", PollingInterval: "notaduration"}, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pollingInterval")
}

func TestQueueURLToARN(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"http://localhost:9324/queue/orders", "arn:aws:sqs:us-east-1:000000000000:orders"},
		{"http://localhost:9324/queue/my-queue", "arn:aws:sqs:us-east-1:000000000000:my-queue"},
		{"http://sqs.us-east-1.amazonaws.com/123456789/test", "arn:aws:sqs:us-east-1:000000000000:test"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, queueURLToARN(tc.url), tc.url)
	}
}

func TestBuildSQSPayload_Fields(t *testing.T) {
	msgs := []sqsMessage{
		{MessageID: "msg-1", ReceiptHandle: "rh-1", Body: `{"key":"value"}`},
		{MessageID: "msg-2", ReceiptHandle: "rh-2", Body: "plain text"},
	}
	data, err := buildSQSPayload(msgs, "http://localhost:9324/queue/orders")
	require.NoError(t, err)

	var evt events.SQSEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	require.Len(t, evt.Records, 2)

	assert.Equal(t, "msg-1", evt.Records[0].MessageId)
	assert.Equal(t, "rh-1", evt.Records[0].ReceiptHandle)
	assert.Equal(t, `{"key":"value"}`, evt.Records[0].Body)
	assert.Equal(t, "aws:sqs", evt.Records[0].EventSource)
	assert.Equal(t, "arn:aws:sqs:us-east-1:000000000000:orders", evt.Records[0].EventSourceARN)
	assert.Equal(t, "us-east-1", evt.Records[0].AWSRegion)
}

func TestSQSTrigger_ReceivesAndInvokes(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	// Fake SQS server: return one message on first call, empty on subsequent.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Return one message.
			resp := sqsReceiveResponse{
				Messages: []struct {
					MessageID     string `xml:"MessageId"`
					ReceiptHandle string `xml:"ReceiptHandle"`
					Body          string `xml:"Body"`
				}{{MessageID: "m1", ReceiptHandle: "rh1", Body: "hello"}},
			}
			data, _ := xml.Marshal(resp)
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(data)
			return
		}
		// Empty response (no messages).
		data, _ := xml.Marshal(sqsReceiveResponse{})
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	invoked := make(chan []byte, 1)
	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, p []byte) ([]byte, error) {
			invoked <- p
			return []byte(`"ok"`), nil
		})

	trigger := &sqsTrigger{
		base:            newBase(t, "svc", sched),
		queueURL:        srv.URL,
		batchSize:       10,
		pollingInterval: 30 * time.Millisecond,
		client:          &http.Client{Timeout: 2 * time.Second},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- trigger.Start(ctx) }()

	select {
	case payload := <-invoked:
		var evt events.SQSEvent
		require.NoError(t, json.Unmarshal(payload, &evt))
		assert.Equal(t, "m1", evt.Records[0].MessageId)
		assert.Equal(t, "hello", evt.Records[0].Body)
	case <-time.After(3 * time.Second):
		t.Fatal("SQS trigger did not invoke Lambda within timeout")
	}

	cancel()
	<-done
}

// ─────────────────────────────────────────────────────────────────────────────
// S3 trigger
// ─────────────────────────────────────────────────────────────────────────────

func TestFsEventToS3_Create(t *testing.T) {
	name, op := fsEventToS3(0x01) // fsnotify.Create == 1
	assert.Equal(t, "s3:ObjectCreated:Put", name)
	assert.NotEqual(t, 0, op)
}

func TestFsEventToS3_Remove(t *testing.T) {
	name, op := fsEventToS3(0x04) // fsnotify.Remove == 4 (1<<2)
	assert.Equal(t, "s3:ObjectRemoved:Delete", name)
	assert.NotEqual(t, 0, op)
}

func TestFsEventToS3_Chmod_Ignored(t *testing.T) {
	name, op := fsEventToS3(0x10) // fsnotify.Chmod == 16 (1<<4)
	assert.Empty(t, name)
	assert.Equal(t, 0, int(op))
}

func TestShouldFire_ExactMatch(t *testing.T) {
	s := &s3Trigger{eventNames: []string{"s3:ObjectCreated:Put"}}
	assert.True(t, s.shouldFire("s3:ObjectCreated:Put"))
	assert.False(t, s.shouldFire("s3:ObjectRemoved:Delete"))
}

func TestShouldFire_WildcardMatch(t *testing.T) {
	s := &s3Trigger{eventNames: []string{"s3:ObjectCreated:*"}}
	assert.True(t, s.shouldFire("s3:ObjectCreated:Put"))
	assert.True(t, s.shouldFire("s3:ObjectCreated:CompleteMultipartUpload"))
	assert.False(t, s.shouldFire("s3:ObjectRemoved:Delete"))
}

func TestShouldFire_MultiplePatterns(t *testing.T) {
	s := &s3Trigger{eventNames: []string{"s3:ObjectCreated:*", "s3:ObjectRemoved:*"}}
	assert.True(t, s.shouldFire("s3:ObjectCreated:Put"))
	assert.True(t, s.shouldFire("s3:ObjectRemoved:Delete"))
	assert.False(t, s.shouldFire("s3:Replication:OperationMissedThreshold"))
}

func TestFsPathToS3Key(t *testing.T) {
	root := "/data/bucket"
	cases := []struct {
		file string
		want string
	}{
		{"/data/bucket/image.jpg", "image.jpg"},
		{"/data/bucket/sub/dir/file.txt", "sub/dir/file.txt"},
		{"/data/bucket/a", "a"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, fsPathToS3Key(root, tc.file), tc.file)
	}
}

func TestBuildS3Payload_Fields(t *testing.T) {
	data, err := buildS3Payload("my-bucket", "images/photo.jpg", "s3:ObjectCreated:Put", "/data/images/photo.jpg")
	require.NoError(t, err)

	var evt events.S3Event
	require.NoError(t, json.Unmarshal(data, &evt))
	require.Len(t, evt.Records, 1)

	r := evt.Records[0]
	assert.Equal(t, "aws:s3", r.EventSource)
	assert.Equal(t, "2.1", r.EventVersion)
	assert.Equal(t, "s3:ObjectCreated:Put", r.EventName)
	assert.Equal(t, "my-bucket", r.S3.Bucket.Name)
	assert.Equal(t, "arn:aws:s3:::my-bucket", r.S3.Bucket.Arn)
	assert.Equal(t, "images%2Fphoto.jpg", r.S3.Object.Key) // URL-escaped
	assert.NotEmpty(t, r.S3.Object.ETag)
	assert.NotEmpty(t, r.S3.Object.Sequencer)
}

func TestS3Trigger_FiresOnFileCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	dir := t.TempDir()
	invoked := make(chan []byte, 1)
	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, p []byte) ([]byte, error) {
			invoked <- p
			return []byte(`"ok"`), nil
		}).AnyTimes()

	src := &s3Trigger{
		base:       newBase(t, "svc", sched),
		localPath:  dir,
		bucket:     "test-bucket",
		eventNames: []string{"s3:ObjectCreated:*"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- src.Start(ctx) }()

	// Give the watcher time to start.
	time.Sleep(100 * time.Millisecond)
	testFile := filepath.Join(dir, "upload.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello"), 0644))

	select {
	case payload := <-invoked:
		var evt events.S3Event
		require.NoError(t, json.Unmarshal(payload, &evt))
		assert.Equal(t, "test-bucket", evt.Records[0].S3.Bucket.Name)
		assert.Contains(t, evt.Records[0].EventName, "s3:ObjectCreated")
	case <-time.After(3 * time.Second):
		t.Fatal("S3 trigger did not invoke Lambda after file creation")
	}

	cancel()
	<-done
}

func TestS3Trigger_DoesNotFireOnFilteredEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	// Expect zero invocations.
	sched.EXPECT().Invoke(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	dir := t.TempDir()
	src := &s3Trigger{
		base:       newBase(t, "svc", sched),
		localPath:  dir,
		bucket:     "test-bucket",
		eventNames: []string{"s3:ObjectRemoved:*"}, // only delete events
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- src.Start(ctx) }()

	time.Sleep(80 * time.Millisecond)
	// Create a file — should NOT trigger because filter is remove-only.
	_ = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)

	<-done // context timeout stops Start
}

func TestS3Trigger_InvalidPath_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	src := &s3Trigger{
		base:       newBase(t, "svc", sched),
		localPath:  "/this/path/does/not/exist",
		bucket:     "b",
		eventNames: []string{"s3:ObjectCreated:*"},
	}
	err := src.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to watch")
}

// ─────────────────────────────────────────────────────────────────────────────
// SNS trigger
// ─────────────────────────────────────────────────────────────────────────────

func TestNewSNS_DefaultPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newSNS(config.Trigger{TopicARN: "arn:aws:sns:local:000:topic"}, b)
	require.NoError(t, err)
	assert.Equal(t, snsDefaultPort, src.(*snsTrigger).port)
}

func TestNewSNS_CustomPort(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newSNS(config.Trigger{TopicARN: "arn:aws:sns:local:000:topic", SNSEndpointPort: 3000}, b)
	require.NoError(t, err)
	assert.Equal(t, 3000, src.(*snsTrigger).port)
}

func TestBuildSNSPayload_Fields(t *testing.T) {
	pub := snsPublishRequest{
		Message: "hello world",
		Subject: "test subject",
	}
	data, err := buildSNSPayload(pub, "arn:aws:sns:local:000:topic", "msg-id-123")
	require.NoError(t, err)

	var evt events.SNSEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	require.Len(t, evt.Records, 1)

	r := evt.Records[0]
	assert.Equal(t, "1.0", r.EventVersion)
	assert.Equal(t, "aws:sns", r.EventSource)
	assert.Equal(t, "arn:aws:sns:local:000:topic", r.SNS.TopicArn)
	assert.Equal(t, "msg-id-123", r.SNS.MessageID)
	assert.Equal(t, "hello world", r.SNS.Message)
	assert.Equal(t, "test subject", r.SNS.Subject)
	assert.Equal(t, "Notification", r.SNS.Type)
}

func TestBuildSNSPayload_WithMessageAttributes(t *testing.T) {
	pub := snsPublishRequest{
		Message: "msg",
		MessageAttributes: map[string]struct {
			DataType    string `json:"DataType"`
			StringValue string `json:"StringValue"`
		}{
			"color": {DataType: "String", StringValue: "red"},
		},
	}
	data, err := buildSNSPayload(pub, "arn:aws:sns:local:000:topic", "id")
	require.NoError(t, err)

	var evt events.SNSEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	attr, ok := evt.Records[0].SNS.MessageAttributes["color"]
	require.True(t, ok)
	attrMap, ok := attr.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "String", attrMap["Type"])
	assert.Equal(t, "red", attrMap["Value"])
}

func TestSNSTrigger_HandlePublish_ValidRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	invoked := make(chan []byte, 1)
	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, p []byte) ([]byte, error) {
			invoked <- p
			return []byte(`"ok"`), nil
		}).AnyTimes()

	trigger := &snsTrigger{
		base:     newBase(t, "svc", sched),
		topicARN: "arn:aws:sns:local:000:topic",
		port:     0,
	}

	body := `{"Message":"hello","Subject":"subj"}`
	req := httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader(body))
	w := httptest.NewRecorder()
	trigger.handlePublish(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["MessageId"])

	// Wait for async invoke.
	select {
	case payload := <-invoked:
		var evt events.SNSEvent
		require.NoError(t, json.Unmarshal(payload, &evt))
		assert.Equal(t, "hello", evt.Records[0].SNS.Message)
	case <-time.After(2 * time.Second):
		t.Fatal("SNS trigger did not invoke Lambda")
	}
}

func TestSNSTrigger_HandlePublish_NonPost_Returns405(t *testing.T) {
	trigger := &snsTrigger{base: base{}}
	req := httptest.NewRequest(http.MethodGet, "/publish", nil)
	w := httptest.NewRecorder()
	trigger.handlePublish(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSNSTrigger_HandlePublish_InvalidJSON_Returns400(t *testing.T) {
	trigger := &snsTrigger{base: base{logger: logrus.NewEntry(logrus.New())}}
	req := httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	trigger.handlePublish(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─────────────────────────────────────────────────────────────────────────────
// DynamoDB Streams trigger
// ─────────────────────────────────────────────────────────────────────────────

func TestNewDynamoDBStream_DefaultStartingPosition(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newDynamoDBStream(config.Trigger{
		StreamARN:        "arn:...",
		DynamoDBEndpoint: "http://localhost:8000",
	}, b)
	require.NoError(t, err)
	assert.Equal(t, dynamoDefaultStartingPos, src.(*dynamoDBStreamTrigger).startingPos)
}

func TestNewDynamoDBStream_TrimsTrailingSlash(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	b := newBase(t, "svc", sched)

	src, err := newDynamoDBStream(config.Trigger{
		StreamARN:        "arn:...",
		DynamoDBEndpoint: "http://localhost:8000/",
	}, b)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8000", src.(*dynamoDBStreamTrigger).endpoint)
}

func TestBuildDynamoDBPayload_Fields(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"eventName":"INSERT","eventSource":"aws:dynamodb","dynamodb":{"Keys":{"id":{"S":"abc"}}}}`),
		json.RawMessage(`{"eventName":"REMOVE","dynamodb":{}}`),
	}
	data, err := buildDynamoDBPayload(raw, "arn:aws:dynamodb:local:000:table/t/stream/2024")
	require.NoError(t, err)

	var evt events.DynamoDBEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	require.Len(t, evt.Records, 2)

	assert.Equal(t, "INSERT", evt.Records[0].EventName)
	assert.Equal(t, "aws:dynamodb", evt.Records[0].EventSource)
	assert.Equal(t, "arn:aws:dynamodb:local:000:table/t/stream/2024", evt.Records[0].EventSourceArn)
	assert.Equal(t, "REMOVE", evt.Records[1].EventName)
	// EventSource backfilled for record that had empty source.
	assert.Equal(t, "aws:dynamodb", evt.Records[1].EventSource)
}

func TestBuildDynamoDBPayload_SkipsMalformedRecords(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"eventName":"INSERT","dynamodb":{}}`),
		json.RawMessage(`not valid json`),
		json.RawMessage(`{"eventName":"REMOVE","dynamodb":{}}`),
	}
	data, err := buildDynamoDBPayload(raw, "arn:...")
	require.NoError(t, err)

	var evt events.DynamoDBEvent
	require.NoError(t, json.Unmarshal(data, &evt))
	// Malformed record skipped; 2 valid records remain.
	assert.Len(t, evt.Records, 2)
}

func TestDynamoDBTrigger_DoRequest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"resource not found"}`)
	}))
	defer srv.Close()

	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	d := &dynamoDBStreamTrigger{
		base:     newBase(t, "svc", sched),
		endpoint: srv.URL,
		client:   &http.Client{Timeout: 2 * time.Second},
	}

	_, err := d.doRequest(context.Background(), "SomeTarget", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DynamoDB API error (500)")
}

func TestDynamoDBTrigger_InitShards_AndPoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	invoked := make(chan []byte, 1)
	sched.EXPECT().
		Invoke(gomock.Any(), "svc", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, p []byte) ([]byte, error) {
			invoked <- p
			return []byte(`"ok"`), nil
		}).AnyTimes()

	callSeq := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Header.Get("X-Amz-Target")
		callSeq++
		switch {
		case strings.Contains(target, "DescribeStream"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"StreamDescription":{"Shards":[{"ShardId":"shard-0"}]}}`)
		case strings.Contains(target, "GetShardIterator"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"ShardIterator":"iter-abc"}`)
		case strings.Contains(target, "GetRecords") && callSeq <= 4:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"NextShardIterator":"iter-next","Records":[{"eventName":"INSERT","dynamodb":{}}]}`)
		default:
			// Empty response after first batch.
			_, _ = fmt.Fprint(w, `{"NextShardIterator":"","Records":[]}`)
		}
	}))
	defer srv.Close()

	d := &dynamoDBStreamTrigger{
		base:            newBase(t, "svc", sched),
		streamARN:       "arn:aws:dynamodb:local:000:table/t/stream/2024",
		endpoint:        srv.URL,
		startingPos:     "LATEST",
		pollingInterval: 30 * time.Millisecond,
		client:          &http.Client{Timeout: 2 * time.Second},
		shardIterators:  make(map[string]string),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Start(ctx) }()

	select {
	case payload := <-invoked:
		var evt events.DynamoDBEvent
		require.NoError(t, json.Unmarshal(payload, &evt))
		assert.Equal(t, "INSERT", evt.Records[0].EventName)
	case <-time.After(3 * time.Second):
		t.Fatal("DynamoDB trigger did not invoke Lambda within timeout")
	}

	cancel()
	<-done
}

// ─────────────────────────────────────────────────────────────────────────────
// UnknownTriggerTypeError
// ─────────────────────────────────────────────────────────────────────────────

func TestUnknownTriggerTypeError_Message(t *testing.T) {
	err := &UnknownTriggerTypeError{Type: "kafka"}
	assert.Equal(t, "unknown trigger type: kafka", err.Error())
}
