package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/sirupsen/logrus"
)

func NewRouter(logger *logrus.Entry) RouterInterface {
	return &Router{
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger.WithField("component", "router"),
	}
}

func (r *Router) SendRequest(
	ctx context.Context,
	url string,
	headers map[string]string,
	payload []byte,
) ([]byte, int, error) {
	startTime := time.Now()
	serviceName := ctx.Value("service").(string)
	logger := r.logger.WithFields(logrus.Fields{"service": serviceName, "url": url})
	logger.Info("sending request to service")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		r.logger.WithError(err).Error("failed to create http request")
		return nil, http.StatusInternalServerError, fmt.Errorf("router failed to create http request: %w", err)
	}
	// Add headers
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		logger.WithError(err).Error("failed to send http request")
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return nil, http.StatusRequestTimeout, simlaerrors.NewTimeoutError(serviceName)
		case context.Canceled:
			return nil, http.StatusRequestTimeout, fmt.Errorf("router: request canceled: %w", ctx.Err())
		default:
			return nil, http.StatusInternalServerError, simlaerrors.NewConnectionError(serviceName)
		}
	}

	defer resp.Body.Close()

	logger.WithField("status_code", resp.StatusCode).Info("received response from service")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Error("failed to read response body")
		return nil, resp.StatusCode, fmt.Errorf("router: failed to read response body: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("service returned non-2xx response")
		return nil, resp.StatusCode, simlaerrors.NewServiceInvocationError(serviceName, resp.StatusCode, string(body))
	}
	duration := time.Since(startTime)
	logger.WithField("duration", duration).Info("request completed successfully")
	return body, resp.StatusCode, nil
}
