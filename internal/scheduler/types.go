//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_scheduler.go -package=mocks SchedulerInterface,RouterInterface

package scheduler

import (
	"context"
	"net/http"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/health"
	"github.com/nyambati/simla/internal/registry"
	"github.com/nyambati/simla/internal/runtime"
	"github.com/sirupsen/logrus"
)

type RouterInterface interface {
	SendRequest(
		ctx context.Context,
		url string,
		headers map[string]string,
		payload []byte,
	) (response []byte, statusCode int, err error)
}

type SchedulerInterface interface {
	Invoke(ctx context.Context, serviceName string, payload []byte) ([]byte, error)
	StartService(ctx context.Context, serviceName string) error
	StopService(ctx context.Context, serviceName string) error
}

type Router struct {
	client *http.Client
	logger *logrus.Entry
}

type Scheduler struct {
	registry registry.ServiceRegistryInterface
	health   health.HealthCheckerInterface
	router   RouterInterface
	logger   *logrus.Entry
	config   *config.Config
	runtime  runtime.RuntimeInterface
}
