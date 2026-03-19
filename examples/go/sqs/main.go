package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// BatchItemFailure is returned to report partial batch failures.
type BatchItemFailure struct {
	ItemIdentifier string `json:"itemIdentifier"`
}

// Response enables partial batch failure reporting so successfully processed
// messages are deleted while failed ones return to the queue.
type Response struct {
	BatchItemFailures []BatchItemFailure `json:"batchItemFailures"`
}

// Handler processes a batch of SQS messages.
func Handler(ctx context.Context, event events.SQSEvent) (Response, error) {
	log.Printf("Received %d SQS message(s)", len(event.Records))

	var failed []BatchItemFailure

	for _, record := range event.Records {
		log.Printf("Processing message %s from %s", record.MessageId, record.EventSourceARN)

		// The body may be raw JSON or a plain string.
		var body interface{}
		if err := json.Unmarshal([]byte(record.Body), &body); err != nil {
			body = record.Body
		}
		log.Printf("Message body: %+v", body)

		// Put your message processing logic here.
		// On failure, append the message ID to report a partial batch failure:
		//   failed = append(failed, BatchItemFailure{ItemIdentifier: record.MessageId})
	}

	log.Printf("Processed %d message(s), %d failure(s)", len(event.Records), len(failed))
	return Response{BatchItemFailures: failed}, nil
}

func main() {
	lambda.Start(Handler)
}
