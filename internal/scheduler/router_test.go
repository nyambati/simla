package scheduler_test

import (
	"context"
	"io"
	"testing"

	"github.com/h2non/gock"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var baseUrl = "http://localhost"
var invocationPath = "/2015-03-31/functions/function/invocations"
var payload = []byte("test payload")

func TestRouter(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		statusCode int
		expected   string
		wantErr    bool
	}{
		{
			name:       "TestValidRouteWithSuccessfulResponse",
			statusCode: 200,
			expected:   string(payload),
			wantErr:    false,
		},
		{
			name:       "TestValidRouteWithFailedResponse",
			statusCode: 500,
			wantErr:    true,
		},
		{
			name:       "TestInvalidRoute",
			path:       "/invalid",
			statusCode: 500,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		defer gock.Off()

		gock.
			New(baseUrl).
			Post(invocationPath).
			Reply(tt.statusCode).
			BodyString(string(payload))

		t.Run(tt.name, func(t *testing.T) {
			if tt.path == "" {
				tt.path = invocationPath
			}
			logger := logrus.NewEntry(&logrus.Logger{Out: io.Discard})
			router := scheduler.NewRouter(logger)
			url := baseUrl + tt.path
			ctx := context.WithValue(context.Background(), "service", "test")
			headers := map[string]string{}

			payload, statusCode, err := router.SendRequest(ctx, url, headers, payload)
			if err != nil && !tt.wantErr {
				t.Errorf("expected no error, got=%v", err)
			}

			assert.Equal(t, tt.expected, string(payload))
			assert.Equal(t, tt.statusCode, statusCode)
		})
	}
}
