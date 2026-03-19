package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Handler processes SNS notifications.
func Handler(ctx context.Context, event events.SNSEvent) (map[string]interface{}, error) {
	log.Printf("Received %d SNS notification(s)", len(event.Records))

	for _, record := range event.Records {
		sns := record.SNS
		log.Printf("SNS message %s from topic %s", sns.MessageID, sns.TopicArn)
		log.Printf("Subject: %s", sns.Subject)

		// The message body may itself be JSON — try to parse it.
		var message interface{}
		if err := json.Unmarshal([]byte(sns.Message), &message); err != nil {
			message = sns.Message
		}
		log.Printf("Message: %+v", message)
		log.Printf("Attributes: %+v", sns.MessageAttributes)

		// Put your notification handling logic here — send emails, push
		// notifications, fan out to other services, etc.
	}

	return map[string]interface{}{
		"status":    "ok",
		"processed": len(event.Records),
	}, nil
}

func main() {
	lambda.Start(Handler)
}
