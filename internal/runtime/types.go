//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_runtime.go -package=mocks RuntimeInterface
package runtime

import (
	"context"

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
}
