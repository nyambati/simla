// Package trigger provides event-source implementations that poll or listen for
// external events and invoke Lambda services through the scheduler.
package trigger

import (
	"context"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

// Source is the common interface every trigger implementation must satisfy.
// Start blocks until ctx is cancelled or an unrecoverable error occurs.
type Source interface {
	Start(ctx context.Context) error
}

// New constructs the correct Source for the given trigger configuration.
// serviceName identifies which Lambda service to invoke when the trigger fires.
func New(
	trig config.Trigger,
	serviceName string,
	sched scheduler.SchedulerInterface,
	logger *logrus.Entry,
) (Source, error) {
	base := base{
		serviceName: serviceName,
		scheduler:   sched,
		logger:      logger,
	}

	switch trig.Type {
	case config.TriggerTypeSchedule:
		return newSchedule(trig, base)
	case config.TriggerTypeSQS:
		return newSQS(trig, base)
	case config.TriggerTypeS3:
		return newS3(trig, base)
	case config.TriggerTypeSNS:
		return newSNS(trig, base)
	case config.TriggerTypeDynamoDBStreams:
		return newDynamoDBStream(trig, base)
	default:
		return nil, &UnknownTriggerTypeError{Type: string(trig.Type)}
	}
}

// base holds the fields shared by all trigger implementations.
type base struct {
	serviceName string
	scheduler   scheduler.SchedulerInterface
	logger      *logrus.Entry
}

// invoke is a convenience helper: it puts the service name in ctx and calls
// scheduler.Invoke, logging both the trigger event and any invocation error.
func (b *base) invoke(ctx context.Context, payload []byte) {
	ctx = context.WithValue(ctx, "service", b.serviceName)
	resp, err := b.scheduler.Invoke(ctx, b.serviceName, payload)
	if err != nil {
		b.logger.WithError(err).Errorf("trigger invocation failed for service %s", b.serviceName)
		return
	}
	b.logger.WithField("response", string(resp)).Debugf("trigger invocation succeeded for service %s", b.serviceName)
}

// UnknownTriggerTypeError is returned when the config specifies a type that
// has no registered implementation.
type UnknownTriggerTypeError struct {
	Type string
}

func (e *UnknownTriggerTypeError) Error() string {
	return "unknown trigger type: " + e.Type
}
