package runtime

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	simlaerrors "github.com/nyambati/simla/internal/errors"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

const (
	InternalPort = "8080"
	OS           = "linux"
)

func NewRuntime(config *RuntimeConfig, logger *logrus.Logger) (RuntimeInterface, error) {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		config: config,
		client: client,
		logger: logger.WithField("component", "runtime"),
	}, nil
}

// StartContainer creates a new container for the runtime configuration and starts it.
// It pulls the required Docker image, creates a new container, and starts it.
// It returns the container ID or an error if any part of the process fails.
func (r *Runtime) StartContainer(ctx context.Context) (containerID string, err error) {
	if err = r.pullImage(ctx); err != nil {
		return "", fmt.Errorf("pulling image failed: %w", err)
	}

	containerID, err = r.createContainer(ctx)
	if err != nil {
		return "", fmt.Errorf("creating container failed: %w", err)
	}

	if err = r.startContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("starting container failed: %w", err)
	}

	return containerID, nil
}

// helper functions

func (r *Runtime) pullImage(ctx context.Context) error {
	r.config.Image = strings.TrimSpace(r.config.Image)
	if r.config.Image == "" {
		return simlaerrors.NewRuntimeConfigError("image field is empty")
	}

	logger := r.logger.WithField("image", r.config.Image)
	// First, check if image already exists
	images, err := r.client.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	for _, image := range images {
		if slices.Contains(image.RepoTags, r.config.Image) {
			return nil
		}
	}

	// Pull image if not found
	reader, err := r.client.ImagePull(ctx, r.config.Image, image.PullOptions{
		Platform: fmt.Sprintf("%s/%s", OS, r.config.Architecture),
	})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", r.config.Image, err)
	}

	defer reader.Close()

	// Drain output (Docker API requires this)
	_, _ = io.Copy(io.Discard, reader)
	logger.WithField("image", r.config.Image).Info("image pulled successfully")
	return nil
}

func (r *Runtime) createContainer(ctx context.Context) (string, error) {
	absCodePath, err := filepath.Abs(r.config.CodePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve code path: %w", err)
	}

	containerConfig := &container.Config{
		Image:        r.config.Image,
		Cmd:          r.config.Cmd,
		Entrypoint:   r.config.Entrypoint,
		Env:          formatEnvVars(r.config.Environment),
		ExposedPorts: nat.PortSet{nat.Port("8080/tcp"): struct{}{}},
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: absCodePath,
				Target: "/var/task",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(InternalPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: r.config.Port,
				},
			},
		},
	}

	networkingConfig := &network.NetworkingConfig{}

	resp, err := r.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkingConfig,
		toV1Platform(r.config.Architecture),
		r.config.Name,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

// toV1Platform converts an architecture string to a v1.Platform pointer.
// It assigns a predefined OS constant and the provided architecture.

func toV1Platform(arch string) *v1.Platform {
	return &v1.Platform{
		OS:           OS,
		Architecture: arch,
	}
}

// formatEnvVars formats environment variables for use in Docker API.
//
// For example, given a map of environment variables {"FOO": "bar", "BAZ": "qux"},
// it will return a slice of strings {"FOO=bar", "BAZ=qux"}.
func formatEnvVars(env map[string]string) []string {
	variables := make([]string, 0, len(env))
	for key, value := range env {
		variables = append(variables, fmt.Sprintf("%s=%s", key, value))
	}
	return variables
}

// startContainer starts a container with the given ID.
//
// It takes a context and container ID as input, and returns an error if any part of the process fails.
//
// The method first logs a message indicating that it is starting the container, and then attempts to start the container using the Docker API.
// If the operation is successful, it logs a message indicating that the container was started successfully.
// If the operation fails, it returns an error with the message "error starting container: <error>".
func (r *Runtime) startContainer(ctx context.Context, containerID string) error {
	logger := r.logger.WithField("container_id", containerID)
	logger.Info("starting container")
	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}
	logger.Info("container started successfully")
	return nil
}

// StopContainer stops a container with the given ID.
//
// It takes a context and container ID as input, and returns an error if any part of the process fails.
//
// The method first logs a message indicating that it is stopping the container,
// and then attempts to stop the container using the Docker API with a 5-second timeout.
// If the operation is successful, it logs a message indicating that the container was stopped successfully.
// If the operation fails, it returns an error with the message "failed to stop container: <error>".
func (r *Runtime) StopContainer(ctx context.Context, containerID string) error {
	stopTimeout := 5
	logger := r.logger.WithField("container_id", containerID)
	logger.Info("stopping container")

	if err := r.client.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &stopTimeout,
	}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	logger.Info("container stopped successfully")
	return nil
}

// DeleteContainer removes a container with the specified ID.
//
// It takes a context and a container ID as input, and returns an error if any part of the process fails.
// The method logs a message indicating that it is deleting the container, and then attempts to remove
// the container using the Docker API with options to remove volumes, force removal, and remove links.
// If the operation is successful, it logs a message indicating that the container was deleted successfully.
// If the operation fails, it returns an error with the message "failed to stop container: <error>".

func (r *Runtime) DeleteContainer(ctx context.Context, containerID string) error {
	logger := r.logger.WithField("container_id", containerID)
	logger.Info("deleting container")

	err := r.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
		RemoveLinks:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	logger.Info("container stopped successfully")
	return nil
}
