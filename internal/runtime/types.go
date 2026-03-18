//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_runtime.go -package=mocks RuntimeInterface
package runtime

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/client"
	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
)

type Runtime struct {
	client   *client.Client
	registry registry.ServiceRegistryInterface
	logger   *logrus.Entry
}

type RuntimeConfig struct {
	Name         string
	Runtime      string
	Image        string
	Architecture string
	CodePath     string
	Cmd          []string
	Entrypoint   []string
	Environment  map[string]string
	Port         string
}

type RuntimeInterface interface {
	StartContainer(ctx context.Context, config *RuntimeConfig) (containerID string, err error)
	StopContainer(ctx context.Context, containerID string) error
	DeleteContainer(ctx context.Context, containerID string) error
	GetLogs(ctx context.Context, containerID string, follow bool) (io.ReadCloser, error)
	// StreamStartupLogs tails container logs for the given window duration and
	// emits each line as a structured log entry. It is a best-effort helper
	// called after StartContainer to surface early crash messages.
	StreamStartupLogs(ctx context.Context, containerID string, window time.Duration)
}
