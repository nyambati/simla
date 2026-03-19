package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/env"
	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/nyambati/simla/internal/health"
	"github.com/nyambati/simla/internal/metrics"
	"github.com/nyambati/simla/internal/registry"
	"github.com/nyambati/simla/internal/runtime"
	"github.com/sirupsen/logrus"
)

// GlobalMetrics is the process-wide invocation recorder. It is initialised
// once on startup and read by the `simla status` command.
var GlobalMetrics = metrics.NewRecorder()

var _ SchedulerInterface = (*Scheduler)(nil)

var InvokeHost = "http://localhost:%d/%s"
var InvokeEndpoint = "2015-03-31/functions/function/invocations"

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

	isHealthy, err := s.health.IsHealthy(ctx, service)
	if err != nil {
		return nil, err
	}

	if !isHealthy {
		return nil, simlaerrors.NewServiceInvocationError(serviceName, 500, "Service is not healthy")
	}

	url := fmt.Sprintf(InvokeHost, service.Port, InvokeEndpoint)

	// Set service name into context for Router
	ctx = context.WithValue(ctx, "service", serviceName)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	headers := map[string]string{}

	start := time.Now()
	response, statusCode, err := s.router.SendRequest(ctx, url, headers, payload)
	elapsed := time.Since(start)

	if err != nil {
		GlobalMetrics.Record(serviceName, elapsed, true)
		return nil, simlaerrors.NewServiceInvocationError(serviceName, statusCode, err.Error())
	}

	GlobalMetrics.Record(serviceName, elapsed, false)
	logger.WithField("latency", elapsed).Info("service invoked successfully")
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

	// Resolve environment variables: merge inline map with optional .env file
	// and expand ${VAR} placeholders against the host environment.
	resolvedEnv, err := env.Resolve(svcCfg.Environment, svcCfg.EnvFile)
	if err != nil {
		logger.WithError(err).Warn("failed to resolve environment variables; using inline config only")
		resolvedEnv = svcCfg.Environment
	} else {
		logger.WithField("env", env.Mask(resolvedEnv)).Debug("resolved service environment")
	}

	runtimeConfig := &runtime.RuntimeConfig{
		Name:         serviceName,
		Runtime:      svcCfg.Runtime,
		Image:        svcCfg.Image,
		Architecture: svcCfg.Architecture,
		CodePath:     svcCfg.CodePath,
		Cmd:          svcCfg.Cmd,
		Entrypoint:   svcCfg.Entrypoint,
		Environment:  resolvedEnv,
		Port:         fmt.Sprintf("%d", service.Port),
	}

	runtime, err := runtime.NewRuntime(s.registry, s.logger)
	if err != nil {
		return err
	}

	containerID, err := runtime.StartContainer(ctx, runtimeConfig)
	if err != nil {
		s.registry.UpdateStatus(ctx, serviceName, registry.StatusFailed)
		return err
	}

	if err := s.health.WaitForHealthy(ctx, service); err != nil {
		// Surface any startup logs before returning the error so the developer
		// can see why the container failed to become healthy.
		runtime.StreamStartupLogs(ctx, containerID, 3*time.Second)
		s.registry.UpdateStatus(ctx, serviceName, registry.StatusFailed)
		return err
	}

	// Stream the first few seconds of startup logs so startup messages are
	// visible in the terminal without requiring `simla logs`.
	go runtime.StreamStartupLogs(ctx, containerID, 3*time.Second)

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

func (s *Scheduler) StopAll(ctx context.Context) error {
	services := s.registry.ListServices(ctx)
	for _, svc := range services {
		if svc.Status != registry.StatusRunning {
			continue
		}
		if err := s.StopService(ctx, svc.Name); err != nil {
			s.logger.WithError(err).Warnf("failed to stop service %s during StopAll", svc.Name)
		}
	}
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

	runtime, err := runtime.NewRuntime(s.registry, s.logger)
	if err != nil {
		return err
	}

	s.registry.UpdateStatus(ctx, serviceName, registry.StatusPending)

	if err = runtime.StopContainer(ctx, service.ID); err != nil {
		return err
	}

	s.registry.UpdateStatus(ctx, serviceName, registry.StatusStopped)

	logger.Info("service stopped successfully")

	return nil
}
