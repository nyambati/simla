package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var registryFile = "registry.yaml"

func NewRegistry(logger *logrus.Entry) ServiceRegistryInterface {
	home, err := os.UserHomeDir()
	if err != nil {
		logger.WithError(err).Error("failed to get user home directory")
	}
	return &ServiceRegistry{
		Services:          make(map[string]*Service),
		BasePort:          9000,
		LastAllocatedPort: 9000 - 1,
		logger:            logger.WithField("component", "registry"),
		FilePath:          filepath.Join(home, ".simla", registryFile),
		mutex:             &sync.RWMutex{},
	}
}

func (r *ServiceRegistry) Load(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	logger := r.logger.WithField("path", r.FilePath)
	logger.Info("loading registry from file")

	file, err := os.Open(r.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(strings.TrimSuffix(r.FilePath, registryFile), 0755); err != nil {
				return fmt.Errorf("failed to create registry directory: %w", err)
			}
			r.Services = make(map[string]*Service)
			return nil
		}
		return simlaerrors.NewRegistryLoadError(err.Error())
	}

	defer file.Close()

	decoder := yaml.NewDecoder(file)

	temp := &ServiceRegistry{}
	if err := decoder.Decode(temp); err != nil {
		return simlaerrors.NewRegistryLoadError(err.Error())
	}

	r.LastAllocatedPort = temp.LastAllocatedPort
	r.Services = temp.Services

	if r.Services == nil {
		r.Services = make(map[string]*Service)
	}

	logger.Info("loaded registry from file")
	return nil
}

func (r *ServiceRegistry) Save(ctx context.Context) error {
	r.logger.Info("saving registry to file")

	file, err := os.Create(r.FilePath)
	if err != nil {
		return simlaerrors.NewRegistrySaveError(err.Error())
	}

	defer file.Close()

	encoder := yaml.NewEncoder(file)

	defer encoder.Close()

	if err := encoder.Encode(r); err != nil {
		return simlaerrors.NewRegistrySaveError(err.Error())
	}

	r.logger.Info("saved registry to file")
	return nil
}

func (r *ServiceRegistry) AddService(ctx context.Context, serviceName string) (svc *Service, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.logger.Infof("adding service %s to registry", serviceName)

	port, err := r.allocateServicePort(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	svc = &Service{Port: port, Status: StatusPending}
	r.Services[serviceName] = svc

	if err := r.Save(ctx); err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *ServiceRegistry) GetService(ctx context.Context, name string) (svc *Service, exists bool) {
	if service, exists := r.Services[name]; exists {
		r.logger.Infof("found service %s found in registry", name)
		return service, true
	}
	r.logger.Warn(simlaerrors.NewServiceNotFoundError(name).Error())
	return r.Services[name], false
}

func (r *ServiceRegistry) allocateServicePort(ctx context.Context, serviceName string) (port int, err error) {
	if service, exists := r.GetService(ctx, serviceName); exists {
		logrus.Warnf("service %s already exists in registry", serviceName)
		return service.Port, nil
	}
	nextPort := r.LastAllocatedPort + 1
	r.LastAllocatedPort = nextPort
	return nextPort, nil
}

func (r *ServiceRegistry) ListServices(ctx context.Context) []*Service {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	services := make([]*Service, 0, len(r.Services))
	for _, service := range r.Services {
		services = append(services, service)
	}
	return services
}

func (r *ServiceRegistry) UpdateStatus(ctx context.Context, name string, status Status) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.Services[name].Status = status
}

func (r *ServiceRegistry) UpdateHealth(ctx context.Context, name string, healthy bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.Services[name].Healthy = healthy
}

func (r *ServiceRegistry) UpdateContainerID(ctx context.Context, serviceName, containerID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if service, exists := r.GetService(ctx, serviceName); exists {
		service.ID = containerID
		r.Services[serviceName] = service
	}
	r.Services[serviceName].ID = containerID
	return r.Save(ctx)
}
