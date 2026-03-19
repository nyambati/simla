package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Handler processes S3 object events (creates, updates, deletes).
func Handler(ctx context.Context, event events.S3Event) (map[string]interface{}, error) {
	log.Printf("Received %d S3 event(s)", len(event.Records))

	processed := 0
	for _, record := range event.Records {
		bucket := record.S3.Bucket.Name
		key, err := url.QueryUnescape(record.S3.Object.Key)
		if err != nil {
			key = record.S3.Object.Key
		}
		etag := record.S3.Object.ETag
		eventName := record.EventName

		log.Printf("Event: %s | s3://%s/%s (ETag: %s)", eventName, bucket, key, etag)

		switch {
		case strings.HasPrefix(eventName, "s3:ObjectCreated"):
			log.Printf("New object uploaded: s3://%s/%s — processing...", bucket, key)
			// Put your object processing logic here — thumbnail generation,
			// virus scanning, metadata extraction, etc.

		case strings.HasPrefix(eventName, "s3:ObjectRemoved"):
			log.Printf("Object deleted: s3://%s/%s — cleaning up...", bucket, key)
			// Put your cleanup logic here — remove derived objects, update index, etc.
		}
		processed++
	}

	return map[string]interface{}{
		"status":    "ok",
		"processed": fmt.Sprintf("%d", processed),
	}, nil
}

func main() {
	lambda.Start(Handler)
}
