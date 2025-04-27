package scheduler

import (
	"context"
	"fmt"

	"github.com/nyambati/simla/internal/config"
	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/health"
	"github.com/nyambati/simla/internal/registry"
	"github.com/nyambati/simla/internal/runtime"
	"github.com/sirupsen/logrus"
)

var _ SchedulerInterface = (*Scheduler)(nil)

var InvokeEndpoint = "http://localhost:%d/2015-03-31/functions/function/invocations"

func NewScheduler(config *config.Config, registry registry.ServiceRegistryInterface, logger *logrus.Entry) SchedulerInterface {
	router := NewRouter(logger)
	health := health.NewHealthChecker(logger)
	return &Scheduler{
		registry: registry,
		logger:   logger,
		health:   health,
		router:   router,
		config:   config,
	}
}

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

	svcCfg, exists := s.config.GetService(ctx, serviceName)
	if !exists {
		return simlaerrors.NewServiceNotFoundError(serviceName)
	}

	runtimeConfig := &runtime.RuntimeConfig{
		Name:         serviceName,
		Runtime:      svcCfg.Runtime,
		Image:        svcCfg.Image,
		Architecture: svcCfg.Architecture,
		CodePath:     svcCfg.CodePath,
		Cmd:          svcCfg.Cmd,
		Entrypoint:   svcCfg.Entrypoint,
		Environment:  svcCfg.Environment,
		Port:         fmt.Sprintf("%d", service.Port),
	}

	runtime, err := runtime.NewRuntime(runtimeConfig, s.logger)
	if err != nil {
		return err
	}

	containerID, err := runtime.StartContainer(ctx)
	if err != nil {
		s.registry.UpdateStatus(ctx, serviceName, registry.StatusFailed)
		return err
	}

	if err := s.health.WaitForHealthy(ctx, service); err != nil {
		s.registry.UpdateStatus(ctx, serviceName, registry.StatusFailed)
		return err
	}

	// Mark service as running and healthy
	s.registry.UpdateStatus(ctx, serviceName, registry.StatusRunning)
	s.registry.UpdateHealth(ctx, serviceName, true)

	// Update container ID
	if err := s.registry.UpdateContainerID(ctx, serviceName, containerID); err != nil {
		return err
	}

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
