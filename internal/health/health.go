package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
)

var healthCheckEndpoint = "http://localhost:%d/runtime/invocation/next"

func NewHealthChecker(retryCount int, retryInterval time.Duration, logger *logrus.Entry) HealthCheckerInterface {
	return &HealthChecker{
		client: &http.Client{Timeout: 2 * time.Second},
		logger: logger.WithField("component", "health"),
	}
}

func (hc *HealthChecker) IsHealthy(ctx context.Context, svc *registry.Service) (bool, error) {
	serviceName, ok := ctx.Value("service").(string)
	if !ok {
		return false, simlaerrors.NewHeathCheckFailedError("unknown", "service name not found in context")
	}
	log := hc.logger.WithField("service", serviceName)
	url := fmt.Sprintf(healthCheckEndpoint, svc.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, simlaerrors.NewHeathCheckFailedError(serviceName, err.Error())
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		log.WithError(err).Warn("failed to perform health check request")
		return false, simlaerrors.NewHeathCheckFailedError(serviceName, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		log.Info("service is healthy")
		return true, nil
	}
	log.WithField("status_code", resp.StatusCode).Warn("service returned unhealthy status")
	return false, simlaerrors.NewHeathCheckFailedError(serviceName, resp.Status)
}

func (hc *HealthChecker) WaitForHealthy(ctx context.Context, svc *registry.Service) error {
	serviceName := ctx.Value("service").(string)
	log := hc.logger.WithField("service", serviceName)

	timeoutCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return simlaerrors.NewTimeoutError(serviceName)
		case <-ticker.C:
			healthy, err := hc.IsHealthy(timeoutCtx, svc)
			if err != nil {
				log.WithError(err).Warn("health check attempt failed")
				continue
			}
			if healthy {
				log.Info("service is healthy")
				return nil
			}
		}
	}
}
