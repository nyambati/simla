package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/nyambati/simla/internal/config"
)

const snsDefaultPort = 2772

// snsPublishRequest is the JSON body accepted by the simla SNS Publish endpoint.
// It mirrors the minimal fields of the AWS SNS Publish API.
type snsPublishRequest struct {
	Message           string `json:"Message"`
	Subject           string `json:"Subject"`
	MessageStructure  string `json:"MessageStructure"`
	MessageAttributes map[string]struct {
		DataType    string `json:"DataType"`
		StringValue string `json:"StringValue"`
	} `json:"MessageAttributes"`
}

type snsTrigger struct {
	base
	topicARN string
	port     int
}

func newSNS(trig config.Trigger, b base) (Source, error) {
	if trig.TopicARN == "" {
		return nil, fmt.Errorf("sns trigger for service %s: topicArn is required", b.serviceName)
	}

	port := trig.SNSEndpointPort
	if port == 0 {
		port = snsDefaultPort
	}

	return &snsTrigger{
		base:     b,
		topicARN: trig.TopicARN,
		port:     port,
	}, nil
}

// Start listens on the configured port for SNS Publish HTTP requests.
// Any POST to / or /publish is treated as a message publication and the Lambda
// is invoked with an events.SNSEvent payload.
func (s *snsTrigger) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePublish)
	mux.HandleFunc("/publish", s.handlePublish)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("sns trigger for service %s: cannot listen on port %d: %w",
			s.serviceName, s.port, err)
	}

	srv := &http.Server{Handler: mux}

	s.logger.Infof("sns trigger started for service %s (topicArn=%s, port=%d)",
		s.serviceName, s.topicARN, s.port)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Infof("sns trigger stopping for service %s", s.serviceName)
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *snsTrigger) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var pub snsPublishRequest
	if err := json.Unmarshal(body, &pub); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	msgID := uuid.NewString()
	payload, err := buildSNSPayload(pub, s.topicARN, msgID)
	if err != nil {
		http.Error(w, "failed to build SNS event", http.StatusInternalServerError)
		return
	}

	// Invoke asynchronously so the HTTP response is returned immediately.
	go s.invoke(r.Context(), payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp, _ := json.Marshal(map[string]string{"MessageId": msgID})
	_, _ = w.Write(resp)
}

func buildSNSPayload(pub snsPublishRequest, topicARN, msgID string) ([]byte, error) {
	attrs := make(map[string]interface{})
	for k, v := range pub.MessageAttributes {
		attrs[k] = map[string]string{
			"Type":  v.DataType,
			"Value": v.StringValue,
		}
	}

	evt := events.SNSEvent{
		Records: []events.SNSEventRecord{
			{
				EventVersion:         "1.0",
				EventSource:          "aws:sns",
				EventSubscriptionArn: fmt.Sprintf("%s:simla-subscription", topicARN),
				SNS: events.SNSEntity{
					Signature:         "simla",
					MessageID:         msgID,
					Type:              "Notification",
					TopicArn:          topicARN,
					Subject:           pub.Subject,
					Message:           pub.Message,
					MessageAttributes: attrs,
					Timestamp:         time.Now(),
				},
			},
		},
	}
	return json.Marshal(evt)
}
