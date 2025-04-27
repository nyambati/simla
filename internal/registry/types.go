package registry

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusPending Status = "pending"
	StatusStopped Status = "stopped"
	StatusFailed  Status = "failed"
)

type Service struct {
	Name         string    `yaml:"name"`
	ID           string    `yaml:"id"`
	Port         int       `yaml:"port"`
	Status       Status    `yaml:"-"`
	Healthy      bool      `yaml:"-"`
	LastChecked  time.Time `yaml:"-"`
	FailureCount int       `yaml:"-"`
}

type ServiceRegistry struct {
	BasePort          int                 `yaml:"basePort"`
	LastAllocatedPort int                 `yaml:"lastAllocatedPort"`
	Services          map[string]*Service `yaml:"services"`
	FilePath          string              `yaml:"-"`
	logger            *logrus.Entry
	mutex             *sync.RWMutex
}

type ServiceRegistryInterface interface {
	Load(ctx context.Context) error
	Save(ctx context.Context) error
	AddService(ctx context.Context, serviceName string) (svc *Service, err error)
	GetService(ctx context.Context, name string) (svc *Service, exists bool)
	ListServices(ctx context.Context) []*Service
	UpdateStatus(ctx context.Context, name string, status Status)
	UpdateHealth(ctx context.Context, name string, healthy bool)
	UpdateContainerID(ctx context.Context, name, containerId string) error
}
