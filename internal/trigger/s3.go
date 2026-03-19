package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/nyambati/simla/internal/config"
)

type s3Trigger struct {
	base
	localPath  string
	bucket     string
	eventNames []string // e.g. ["s3:ObjectCreated:*", "s3:ObjectRemoved:*"]
}

func newS3(trig config.Trigger, b base) (Source, error) {
	if trig.LocalPath == "" {
		return nil, fmt.Errorf("s3 trigger for service %s: localPath is required", b.serviceName)
	}
	if trig.Bucket == "" {
		return nil, fmt.Errorf("s3 trigger for service %s: bucket is required", b.serviceName)
	}

	eventNames := trig.Events
	if len(eventNames) == 0 {
		// Default: react to all creates and removes.
		eventNames = []string{"s3:ObjectCreated:*", "s3:ObjectRemoved:*"}
	}

	abs, err := filepath.Abs(trig.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("s3 trigger for service %s: cannot resolve localPath: %w", b.serviceName, err)
	}

	return &s3Trigger{
		base:       b,
		localPath:  abs,
		bucket:     trig.Bucket,
		eventNames: eventNames,
	}, nil
}

func (s *s3Trigger) Start(ctx context.Context) error {
	s.logger.Infof("s3 trigger started for service %s (path=%s, bucket=%s)",
		s.serviceName, s.localPath, s.bucket)

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("s3 trigger: failed to create watcher: %w", err)
	}
	defer fsw.Close()

	if err := fsw.Add(s.localPath); err != nil {
		return fmt.Errorf("s3 trigger: failed to watch %s: %w", s.localPath, err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("s3 trigger stopped for service %s", s.serviceName)
			return nil

		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}

			s3EventName, op := fsEventToS3(event.Op)
			if op == 0 {
				continue // ignored op (e.g. chmod)
			}
			if !s.shouldFire(s3EventName) {
				continue
			}

			key := fsPathToS3Key(s.localPath, event.Name)
			payload, err := buildS3Payload(s.bucket, key, s3EventName, event.Name)
			if err != nil {
				s.logger.WithError(err).Warn("s3: failed to build event payload")
				continue
			}
			s.invoke(ctx, payload)

		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			s.logger.WithError(err).Warn("s3 watcher error")
		}
	}
}

// fsEventToS3 maps an fsnotify op to an S3 event name and returns the raw op
// for filtering. Returns op=0 when the event should be ignored.
func fsEventToS3(op fsnotify.Op) (eventName string, mappedOp fsnotify.Op) {
	switch {
	case op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0:
		return "s3:ObjectCreated:Put", op
	case op&fsnotify.Remove != 0:
		return "s3:ObjectRemoved:Delete", op
	default:
		return "", 0
	}
}

// shouldFire checks if s3EventName matches any of the configured event filters.
// Wildcards are supported at the suffix (e.g. "s3:ObjectCreated:*").
func (s *s3Trigger) shouldFire(eventName string) bool {
	for _, pattern := range s.eventNames {
		if pattern == eventName {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(eventName, prefix) {
				return true
			}
		}
	}
	return false
}

// fsPathToS3Key converts an absolute file path to a relative S3 key by
// stripping the watched root directory prefix.
func fsPathToS3Key(root, filePath string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return filepath.Base(filePath)
	}
	return filepath.ToSlash(rel)
}

func buildS3Payload(bucket, key, eventName, filePath string) ([]byte, error) {
	now := time.Now()
	evt := events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventVersion: "2.1",
				EventSource:  "aws:s3",
				AWSRegion:    "us-east-1",
				EventTime:    now,
				EventName:    eventName,
				S3: events.S3Entity{
					SchemaVersion:   "1.0",
					ConfigurationID: "simla-s3-trigger",
					Bucket: events.S3Bucket{
						Name: bucket,
						OwnerIdentity: events.S3UserIdentity{
							PrincipalID: "simla",
						},
						Arn: fmt.Sprintf("arn:aws:s3:::%s", bucket),
					},
					Object: events.S3Object{
						Key:       url.PathEscape(key),
						ETag:      uuid.NewString(),
						Sequencer: fmt.Sprintf("%016X", now.UnixNano()),
					},
				},
			},
		},
	}
	return json.Marshal(evt)
}
