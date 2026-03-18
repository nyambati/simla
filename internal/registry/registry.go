package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	simlaerrors "github.com/nyambati/simla/internal/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var registryFile = "registry.yaml"

func NewRegistry(logger *logrus.Entry) (ServiceRegistryInterface, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	return &ServiceRegistry{
		Services:          make(map[string]*Service),
		BasePort:          9000,
		LastAllocatedPort: 9000 - 1,
		logger:            logger.WithField("component", "registry"),
		FilePath:          filepath.Join(home, ".simla", registryFile),
		mutex:             &sync.RWMutex{},
	}, nil
}

func (r *ServiceRegistry) Load(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	logger := r.logger.WithField("path", r.FilePath)
	logger.Info("loading registry from file")

	file, err := os.Open(r.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(r.FilePath), 0755); err != nil {
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

	// Backfill Name for entries loaded from older registry files that predate
	// the Name field.
	for name, svc := range r.Services {
		if svc.Name == "" {
			svc.Name = name
		}
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

	// Return the existing entry without a disk write — the service is already
	// registered and its port is stable.
	if existing, exists := r.Services[serviceName]; exists {
		return existing, nil
	}

	r.logger.Infof("adding service %s to registry", serviceName)

	port, err := r.allocateServicePort(serviceName)
	if err != nil {
		return nil, err
	}

	svc = &Service{Name: serviceName, Port: port, Status: StatusPending}
	r.Services[serviceName] = svc

	if err := r.Save(ctx); err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *ServiceRegistry) GetService(ctx context.Context, name string) (svc *Service, exists bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	service, exists := r.Services[name]
	if !exists {
		r.logger.Warn(simlaerrors.NewServiceNotFoundError(name).Error())
	}
	return service, exists
}

func (r *ServiceRegistry) getService(name string) (*Service, bool) {
	service, exists := r.Services[name]
	if !exists {
		r.logger.Warn(simlaerrors.NewServiceNotFoundError(name).Error())
	}
	return service, exists
}

func (r *ServiceRegistry) allocateServicePort(serviceName string) (port int, err error) {
	if service, exists := r.getService(serviceName); exists {
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
	svc, exists := r.Services[name]
	if !exists {
		r.logger.Warnf("UpdateStatus: service %s not found in registry", name)
		return
	}
	svc.Status = status
}

func (r *ServiceRegistry) UpdateHealth(ctx context.Context, name string, healthy bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	svc, exists := r.Services[name]
	if !exists {
		r.logger.Warnf("UpdateHealth: service %s not found in registry", name)
		return
	}
	svc.Healthy = healthy
}

func (r *ServiceRegistry) UpdateContainerID(ctx context.Context, serviceName, containerID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if service, exists := r.getService(serviceName); exists {
		service.ID = containerID
		r.Services[serviceName] = service
		return r.Save(ctx)
	}
	return simlaerrors.NewServiceNotFoundError(serviceName)
}
