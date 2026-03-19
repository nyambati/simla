package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Handler processes scheduled EventBridge invocations.
// It receives an events.CloudWatchEvent and performs periodic work.
func Handler(ctx context.Context, event events.CloudWatchEvent) (map[string]string, error) {
	log.Printf("Scheduled event fired: id=%s time=%s", event.ID, event.Time)
	log.Printf("Source rule: %v", event.Resources)

	// Put your periodic work here — reconciliation, cleanup, reporting, etc.
	result := map[string]string{
		"status":     "ok",
		"firedAt":    fmt.Sprintf("%s", event.Time),
		"eventId":    event.ID,
		"detailType": event.DetailType,
	}

	log.Printf("Schedule handler result: %+v", result)
	return result, nil
}

func main() {
	lambda.Start(Handler)
}
