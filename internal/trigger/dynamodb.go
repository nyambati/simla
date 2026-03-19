package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/nyambati/simla/internal/config"
)

const (
	dynamoDefaultPollingInterval = 1 * time.Second
	dynamoDefaultStartingPos     = "LATEST"
)

type dynamoDBStreamTrigger struct {
	base
	streamARN       string
	endpoint        string
	startingPos     string
	pollingInterval time.Duration
	client          *http.Client

	// mutable polling state — not shared across goroutines
	shardIterators map[string]string // shardID → iteratorToken
}

func newDynamoDBStream(trig config.Trigger, b base) (Source, error) {
	if trig.StreamARN == "" {
		return nil, fmt.Errorf("dynamodb-stream trigger for service %s: streamArn is required", b.serviceName)
	}
	if trig.DynamoDBEndpoint == "" {
		return nil, fmt.Errorf("dynamodb-stream trigger for service %s: dynamodbEndpoint is required", b.serviceName)
	}

	startingPos := trig.StartingPosition
	if startingPos == "" {
		startingPos = dynamoDefaultStartingPos
	}

	return &dynamoDBStreamTrigger{
		base:            b,
		streamARN:       trig.StreamARN,
		endpoint:        strings.TrimRight(trig.DynamoDBEndpoint, "/"),
		startingPos:     startingPos,
		pollingInterval: dynamoDefaultPollingInterval,
		client:          &http.Client{Timeout: 10 * time.Second},
		shardIterators:  make(map[string]string),
	}, nil
}

func (d *dynamoDBStreamTrigger) Start(ctx context.Context) error {
	d.logger.Infof("dynamodb-stream trigger started for service %s (streamArn=%s)",
		d.serviceName, d.streamARN)

	// Discover shards and obtain initial iterators.
	if err := d.initShards(ctx); err != nil {
		return fmt.Errorf("dynamodb-stream trigger for service %s: failed to init shards: %w",
			d.serviceName, err)
	}

	ticker := time.NewTicker(d.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Infof("dynamodb-stream trigger stopped for service %s", d.serviceName)
			return nil
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// dynamoRequest / dynamoResponse helpers use the low-level DynamoDB Streams
// HTTP API with JSON bodies and the DynamoDB-specific Content-Type header.

func (d *dynamoDBStreamTrigger) doRequest(ctx context.Context, target string, reqBody interface{}) ([]byte, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint+"/",
		strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", target)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DynamoDB API error (%d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// initShards calls DescribeStream and GetShardIterator for every shard.
func (d *dynamoDBStreamTrigger) initShards(ctx context.Context) error {
	body, err := d.doRequest(ctx, "DynamoDBStreams_20120810.DescribeStream", map[string]string{
		"StreamArn": d.streamARN,
	})
	if err != nil {
		return err
	}

	var desc struct {
		StreamDescription struct {
			Shards []struct {
				ShardID string `json:"ShardId"`
			} `json:"Shards"`
		} `json:"StreamDescription"`
	}
	if err := json.Unmarshal(body, &desc); err != nil {
		return fmt.Errorf("failed to parse DescribeStream response: %w", err)
	}

	for _, shard := range desc.StreamDescription.Shards {
		iter, err := d.getShardIterator(ctx, shard.ShardID)
		if err != nil {
			d.logger.WithError(err).Warnf("dynamodb-stream: skipping shard %s", shard.ShardID)
			continue
		}
		d.shardIterators[shard.ShardID] = iter
	}
	return nil
}

func (d *dynamoDBStreamTrigger) getShardIterator(ctx context.Context, shardID string) (string, error) {
	body, err := d.doRequest(ctx, "DynamoDBStreams_20120810.GetShardIterator", map[string]string{
		"StreamArn":         d.streamARN,
		"ShardId":           shardID,
		"ShardIteratorType": d.startingPos,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		ShardIterator string `json:"ShardIterator"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.ShardIterator, nil
}

// poll fetches records from every known shard and invokes the Lambda once per
// shard that returned records.
func (d *dynamoDBStreamTrigger) poll(ctx context.Context) {
	for shardID, iter := range d.shardIterators {
		if iter == "" {
			continue
		}

		records, nextIter, err := d.getRecords(ctx, iter)
		if err != nil {
			d.logger.WithError(err).Warnf("dynamodb-stream: GetRecords failed for shard %s", shardID)
			continue
		}

		// Advance the iterator for the next poll.
		d.shardIterators[shardID] = nextIter

		if len(records) == 0 {
			continue
		}

		payload, err := buildDynamoDBPayload(records, d.streamARN)
		if err != nil {
			d.logger.WithError(err).Warn("dynamodb-stream: failed to build event payload")
			continue
		}
		d.invoke(ctx, payload)
	}
}

func (d *dynamoDBStreamTrigger) getRecords(ctx context.Context, iterator string) (
	records []json.RawMessage, nextIterator string, err error,
) {
	body, err := d.doRequest(ctx, "DynamoDBStreams_20120810.GetRecords", map[string]interface{}{
		"ShardIterator": iterator,
		"Limit":         100,
	})
	if err != nil {
		return nil, "", err
	}

	var result struct {
		NextShardIterator string            `json:"NextShardIterator"`
		Records           []json.RawMessage `json:"Records"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse GetRecords response: %w", err)
	}
	return result.Records, result.NextShardIterator, nil
}

// buildDynamoDBPayload wraps raw DynamoDB Streams records in an
// events.DynamoDBEvent envelope. The records are forwarded as-is so the Lambda
// sees the same structure it would receive on AWS.
func buildDynamoDBPayload(rawRecords []json.RawMessage, streamARN string) ([]byte, error) {
	// Unmarshal each record into an events.DynamoDBEventRecord so we can set
	// the EventSourceARN field (not present in the Streams API response).
	eventRecords := make([]events.DynamoDBEventRecord, 0, len(rawRecords))
	for _, raw := range rawRecords {
		var rec events.DynamoDBEventRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue // skip malformed records
		}
		rec.EventSourceArn = streamARN
		if rec.EventSource == "" {
			rec.EventSource = "aws:dynamodb"
		}
		eventRecords = append(eventRecords, rec)
	}

	return json.Marshal(events.DynamoDBEvent{Records: eventRecords})
}
