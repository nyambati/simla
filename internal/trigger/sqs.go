package trigger

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/nyambati/simla/internal/config"
)

const (
	sqsDefaultBatchSize       = 10
	sqsDefaultPollingInterval = time.Second
	sqsMaxWaitSeconds         = "20" // long-poll duration
)

type sqsTrigger struct {
	base
	queueURL        string
	batchSize       int
	pollingInterval time.Duration
	client          *http.Client
}

func newSQS(trig config.Trigger, b base) (Source, error) {
	if trig.QueueURL == "" {
		return nil, fmt.Errorf("sqs trigger for service %s: queueUrl is required", b.serviceName)
	}

	batchSize := trig.BatchSize
	if batchSize <= 0 {
		batchSize = sqsDefaultBatchSize
	}
	if batchSize > 10 {
		batchSize = 10 // SQS maximum
	}

	pollingInterval := sqsDefaultPollingInterval
	if trig.PollingInterval != "" {
		d, err := time.ParseDuration(trig.PollingInterval)
		if err != nil {
			return nil, fmt.Errorf("sqs trigger for service %s: invalid pollingInterval %q: %w",
				b.serviceName, trig.PollingInterval, err)
		}
		pollingInterval = d
	}

	return &sqsTrigger{
		base:            b,
		queueURL:        trig.QueueURL,
		batchSize:       batchSize,
		pollingInterval: pollingInterval,
		client:          &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (s *sqsTrigger) Start(ctx context.Context) error {
	s.logger.Infof("sqs trigger started for service %s (queue=%s, batchSize=%d)",
		s.serviceName, s.queueURL, s.batchSize)

	ticker := time.NewTicker(s.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("sqs trigger stopped for service %s", s.serviceName)
			return nil
		case <-ticker.C:
			messages, err := s.receiveMessages(ctx)
			if err != nil {
				s.logger.WithError(err).Warn("sqs: failed to receive messages")
				continue
			}
			if len(messages) == 0 {
				continue
			}

			payload, err := buildSQSPayload(messages, s.queueURL)
			if err != nil {
				s.logger.WithError(err).Warn("sqs: failed to build event payload")
				continue
			}

			s.invoke(ctx, payload)

			// Delete successfully processed messages.
			for _, msg := range messages {
				if err := s.deleteMessage(ctx, msg.ReceiptHandle); err != nil {
					s.logger.WithError(err).Warnf("sqs: failed to delete message %s", msg.MessageID)
				}
			}
		}
	}
}

// sqsMessage is the minimal subset of an SQS message we need from the XML response.
type sqsMessage struct {
	MessageID     string
	ReceiptHandle string
	Body          string
	Attributes    map[string]string
}

// sqsReceiveResponse is the XML envelope returned by ReceiveMessage.
type sqsReceiveResponse struct {
	XMLName  xml.Name `xml:"ReceiveMessageResponse"`
	Messages []struct {
		MessageID     string `xml:"MessageId"`
		ReceiptHandle string `xml:"ReceiptHandle"`
		Body          string `xml:"Body"`
	} `xml:"ReceiveMessageResult>Message"`
}

func (s *sqsTrigger) receiveMessages(ctx context.Context) ([]sqsMessage, error) {
	params := url.Values{
		"Action":              {"ReceiveMessage"},
		"MaxNumberOfMessages": {fmt.Sprintf("%d", s.batchSize)},
		"WaitTimeSeconds":     {sqsMaxWaitSeconds},
		"Version":             {"2012-11-05"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.queueURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result sqsReceiveResponse
	if err := xml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse SQS response: %w", err)
	}

	messages := make([]sqsMessage, 0, len(result.Messages))
	for _, m := range result.Messages {
		messages = append(messages, sqsMessage{
			MessageID:     m.MessageID,
			ReceiptHandle: m.ReceiptHandle,
			Body:          m.Body,
		})
	}
	return messages, nil
}

func (s *sqsTrigger) deleteMessage(ctx context.Context, receiptHandle string) error {
	params := url.Values{
		"Action":        {"DeleteMessage"},
		"ReceiptHandle": {receiptHandle},
		"Version":       {"2012-11-05"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.queueURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func buildSQSPayload(messages []sqsMessage, queueURL string) ([]byte, error) {
	// Derive a queue ARN-like identifier from the URL.
	queueARN := queueURLToARN(queueURL)

	records := make([]events.SQSMessage, 0, len(messages))
	for _, m := range messages {
		records = append(records, events.SQSMessage{
			MessageId:      m.MessageID,
			ReceiptHandle:  m.ReceiptHandle,
			Body:           m.Body,
			EventSource:    "aws:sqs",
			EventSourceARN: queueARN,
			AWSRegion:      "us-east-1",
			Md5OfBody:      uuid.NewString(), // placeholder
		})
	}

	return json.Marshal(events.SQSEvent{Records: records})
}

// queueURLToARN converts a queue URL to a pseudo-ARN for the event payload.
// e.g. "http://localhost:9324/queue/my-queue" → "arn:aws:sqs:us-east-1:000000000000:my-queue"
func queueURLToARN(queueURL string) string {
	parts := strings.Split(queueURL, "/")
	name := parts[len(parts)-1]
	return fmt.Sprintf("arn:aws:sqs:us-east-1:000000000000:%s", name)
}
