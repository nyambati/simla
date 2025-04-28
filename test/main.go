package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

// Event is the expected input
type Event struct {
	Name string `json:"name"`
}

// Handler is the main Lambda function
func Handler(ctx context.Context, event Event) (string, error) {
	return fmt.Sprintf("Hello, %s!", event.Name), nil
}

func main() {
	lambda.Start(Handler)
}
