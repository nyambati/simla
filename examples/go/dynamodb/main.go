package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Handler processes DynamoDB Streams records.
func Handler(ctx context.Context, event events.DynamoDBEvent) (map[string]interface{}, error) {
	log.Printf("Received %d DynamoDB stream record(s)", len(event.Records))

	for _, record := range event.Records {
		log.Printf("Event: %s | Source: %s", record.EventName, record.EventSourceArn)

		keys, _ := json.Marshal(record.Change.Keys)
		log.Printf("Keys: %s", keys)

		switch record.EventName {
		case string(events.DynamoDBOperationTypeInsert):
			newImage, _ := json.Marshal(record.Change.NewImage)
			log.Printf("INSERT — new item: %s", newImage)
			// Put your insert handling logic here — audit log, downstream notify, etc.

		case string(events.DynamoDBOperationTypeModify):
			oldImage, _ := json.Marshal(record.Change.OldImage)
			newImage, _ := json.Marshal(record.Change.NewImage)
			log.Printf("MODIFY — before: %s", oldImage)
			log.Printf("MODIFY — after:  %s", newImage)
			// Put your update handling logic here — cache invalidation, sync, etc.

		case string(events.DynamoDBOperationTypeRemove):
			oldImage, _ := json.Marshal(record.Change.OldImage)
			log.Printf("REMOVE — deleted item: %s", oldImage)
			// Put your delete handling logic here — cleanup, archival, etc.
		}
	}

	return map[string]interface{}{
		"status":    "ok",
		"processed": len(event.Records),
	}, nil
}

func main() {
	lambda.Start(Handler)
}
