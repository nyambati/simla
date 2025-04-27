package scheduler

import (
	"context"
	"fmt"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/registry"
)

var _ SchedulerInterface = (*Scheduler)(nil)

var InvokeEndpoint = "http://localhost:%d/2015-03-31/functions/function/invocations"

func (s *Scheduler) Invoke(ctx context.Context, serviceName string, payload []byte) ([]byte, error) {
	logger := s.logger.WithField("service", serviceName)

	service, err := s.registry.AddService(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	if service.Status != registry.StatusRunning || !service.Healthy {
		logger.Warn("service not running or unhealthy, starting service")
		if err := s.StartService(ctx, serviceName); err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf(InvokeEndpoint, service.Port)
	// Set service name into context for Router
	ctx = context.WithValue(ctx, "service", serviceName)

	headers := map[string]string{}
	// Call Router
	response, statusCode, err := s.router.SendRequest(ctx, url, headers, payload)
	if err != nil {
		return nil, simlaerrors.NewServiceInvocationError(serviceName, statusCode, err.Error())
	}

	logger.Info("service invoked successfully")
	return response, nil
}

func (s *Scheduler) StartService(ctx context.Context, serviceName string) error {
	logger := s.logger.WithField("service", serviceName)

	service, exists := s.registry.GetService(ctx, serviceName)
	if !exists {
		err := simlaerrors.NewServiceNotFoundError(serviceName)
		logger.WithError(err).Warn("service not found in registry")
		return nil
	}

	if service.Status == registry.StatusRunning {
		logger.Info("service already running")
		return nil
	}

	logger.Info("starting service container")

	// TODO: Here you would normally trigger the container runtime (Docker API)
	// For now, let's simulate starting successfully.

	// Update status to Pending while starting
	s.registry.UpdateStatus(ctx, serviceName, registry.StatusPending)

	// TODO: Start container using your container runtime
	// For MVP, we will simulate immediate start success:

	// Mark service as running and healthy
	s.registry.UpdateStatus(ctx, serviceName, registry.StatusRunning)
	s.registry.UpdateHealth(ctx, serviceName, true)

	logger.Info("service started successfully")

	return nil
}

func (s *Scheduler) StopService(ctx context.Context, serviceName string) error {
	logger := s.logger.WithField("service", serviceName)

	service, exists := s.registry.GetService(ctx, serviceName)
	if !exists {
		logger.Warn("service not found in registry")
		return nil
	}

	if service.Status != registry.StatusRunning {
		logger.Info("service not running")
		return nil
	}

	logger.Info("stopping service container")

	// TODO: Here you would normally trigger the container runtime (Docker API)
	// For now, let's simulate stopping successfully.

	// Update status to Pending while stopping
	s.registry.UpdateStatus(ctx, serviceName, registry.StatusPending)

	// TODO: Stop container using your container runtime
	// For MVP, we will simulate immediate stop success:

	// Mark service as stopped
	s.registry.UpdateStatus(ctx, serviceName, registry.StatusStopped)

	logger.Info("service stopped successfully")

	return nil
}
