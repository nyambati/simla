package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/mocks"
	"github.com/nyambati/simla/internal/registry"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

var cfg = &config.Config{
	Services: map[string]config.Service{
		"test": {
			Runtime:      "go",
			Architecture: "amd64",
			Environment: map[string]string{
				"TEST_ENV": "test",
			},
			Cmd:      []string{"main"},
			CodePath: "../../bin",
		},
	},
}

func TestScheduler_Invoke(t *testing.T) {

	type args struct {
		serviceName string
		port        int
		payload     string
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "TestInvokeValidService",
			args: args{
				serviceName: "test",
				port:        9001,
				payload:     "{\"name\": \"Simla\"}",
			},
			want:    "Simla!",
			wantErr: false,
		},
		{
			name: "TestInvokeInvalidService",
			args: args{
				serviceName: "test-2",
				port:        9001,
				payload:     "{\"name\": \"Simla\"}",
			},
			wantErr: true,
		},
		{
			name: "TestInvokeInvalidPayload",
			args: args{
				serviceName: "test",
				port:        9002,
				payload:     "Simla",
			},
			want:    "SyntaxError",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		deleteContainer(tt.args.serviceName)
		t.Run(tt.name, func(t *testing.T) {
			reg := mocks.NewMockServiceRegistryInterface(gomock.NewController(t))
			logger := logrus.NewEntry(&logrus.Logger{Out: os.Stdout})
			ctx := context.WithValue(context.Background(), "service", tt.args.serviceName)

			// Create mock service registry
			reg.
				EXPECT().
				AddService(gomock.Any(), gomock.Any()).
				Return(&registry.Service{Port: tt.args.port, Status: registry.StatusPending}, nil).AnyTimes()

			reg.
				EXPECT().
				GetService(gomock.Any(), gomock.Any()).
				Return(&registry.Service{Port: tt.args.port}, true).AnyTimes()

			reg.
				EXPECT().
				UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			reg.
				EXPECT().
				UpdateHealth(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			reg.
				EXPECT().
				UpdateContainerID(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			s := NewScheduler(cfg, reg, logger)

			got, err := s.Invoke(ctx, tt.args.serviceName, []byte(tt.args.payload))
			if (err != nil) != tt.wantErr {
				t.Errorf("Scheduler.Invoke() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Contains(t, string(got), tt.want)
		})

		t.Cleanup(func() {
			deleteContainer(tt.args.serviceName)
		})
	}
}

func deleteContainer(name string) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	s, err := c.ContainerList(context.Background(), container.ListOptions{})

	if err != nil {
		logrus.WithError(err).Fatal("failed to list containers")
	}
	for _, ct := range s {
		if strings.Contains(ct.Names[0], name) {
			c.ContainerStop(context.Background(), ct.ID, container.StopOptions{})
			c.ContainerRemove(context.Background(), ct.ID, container.RemoveOptions{})
		}
	}
}
