//go:generate mockgen -source=$GOFILE -destination=../mocks/mock_gateway.go -package=mocks GatewayInterface

package gateway

import (
	"context"

	"github.com/gorilla/mux"
	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

type GatewayInterface interface {
	Start(ctx context.Context) error
}

type APIGateway struct {
	config    *config.APIGateway
	scheduler scheduler.SchedulerInterface
	logger    *logrus.Entry
	router    *mux.Router
}
