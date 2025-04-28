//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_health.go -package=mocks HealthCheckerInterface

package health

import (
	"context"
	"net/http"
	"time"

	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
)

type HealthChecker struct {
	client  *http.Client
	logger  *logrus.Entry
	timeout time.Duration
}

type HealthCheckerInterface interface {
	WaitForHealthy(ctx context.Context, service *registry.Service) error
	IsHealthy(ctx context.Context, service *registry.Service) (bool, error)
}
