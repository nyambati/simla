# Triggers Guide

Triggers invoke Lambda services based on external events. Simla supports multiple event source types that run as background goroutines when you start the server with `simla up`.

## Overview

Triggers are defined in the `triggers` section of each service in `.simla.yaml`:

```yaml
services:
  my-service:
    triggers:
      - type: schedule
        expression: "rate(5 minutes)"
```

All triggers start automatically when you run `simla up`.

## Supported Triggers

| Type | Description |
|------|-------------|
| `schedule` | Time-based invocations (rate/cron) |
| `sqs` | Amazon SQS queue polling |
| `s3` | File system watcher (S3 emulation) |
| `sns` | Simple Notification Service |
| `dynamodb-stream` | DynamoDB Streams polling |

---

## Schedule Trigger

Invokes your Lambda on a schedule using CloudWatch Events-style expressions.

### Configuration

```yaml
services:
  reconciler:
    runtime: go
    codePath: ./services/reconciler
    cmd: ["main"]
    triggers:
      - type: schedule
        # Rate expression (every N units)
        expression: "rate(5 minutes)"
        # OR cron expression
        # expression: "cron(0 12 * * ? *)"
```

### Rate Expressions

`rate(value unit)` where value is a positive integer.

| Expression | Description |
|------------|-------------|
| `rate(1 minute)` | Every minute |
| `rate(5 minutes)` | Every 5 minutes |
| `rate(1 hour)` | Every hour |
| `rate(2 hours)` | Every 2 hours |
| `rate(1 day)` | Every day |

### Cron Expressions

Standard 6-field cron format: `cron(minute hour day-of-month month day-of-week year)`

| Field | Values | Wildcards |
|-------|--------|-----------|
| Minute | 0-59 | , - * / |
| Hour | 0-23 | , - * / |
| Day | 1-31 | , - * / |
| Month | 1-12 | , - * / |
| Day-of-week | 0-6 or SUN-SAT | , - * / |
| Year | 1970-2199 | , - * / |

**Examples:**

```yaml
# Every day at noon UTC
expression: "cron(0 12 * * ? *)"

# Every weekday at 9 AM
expression: "cron(0 9 * * MON-FRI)"

# Every 15 minutes
expression: "cron(0/15 * * * ? *)"

# First day of every month at midnight
expression: "cron(0 0 1 * ? *)"
```

### Event Format

Schedule triggers send an `events.CloudWatchEvent`:

```go
// Go handler example
func Handler(ctx context.Context, event events.CloudWatchEvent) error {
    log.Printf("Scheduled run: %s", event.ID)
    // Your logic here
    return nil
}
```

```python
# Python handler example
import json

def handler(event, context):
    print(f"Scheduled run: {event['id']}")
    # Your logic here
```

---

## SQS Trigger

Polls an SQS-compatible queue and invokes your Lambda with batches of messages.

### Prerequisites

Requires an SQS-compatible queue endpoint. For local development, use [ElasticMQ](https://github.com/softwaremill/elasticmq):

```bash
# Docker
docker run -p 9324:9324 -p 9325:9324 softwaremill/elasticmq

# Or npm
npm install -g elasticmq
elasticmq start
```

### Configuration

```yaml
services:
  order-processor:
    runtime: go
    codePath: ./services/order-processor
    cmd: ["main"]
    triggers:
      - type: sqs
        queueUrl: "http://localhost:9324/queue/orders"
        batchSize: 10                    # Max messages per batch (default: 10, max: 10)
        pollingInterval: "2s"            # How often to poll (default: 1s)
```

### Event Format

SQS triggers send an `events.SQSEvent`:

```go
// Go handler
func Handler(ctx context.Context, event events.SQSEvent) error {
    for _, record := range event.Records {
        log.Printf("Processing message %s: %s", record.MessageId, record.Body)
        
        // Process message...
    }
    return nil
}
```

```python
# Python handler
def handler(event, context):
    for record in event['Records']:
        print(f"Processing message {record['messageId']}: {record['body']}")
        # Process message...
```

### Partial Batch Failures

Return a response with `batchItemFailures` to report failed messages. Successfully processed messages are deleted from the queue.

```go
type BatchItemFailure struct {
    ItemIdentifier string `json:"itemIdentifier"`
}

type Response struct {
    BatchItemFailures []BatchItemFailure `json:"batchItemFailures"`
}

func Handler(ctx context.Context, event events.SQSEvent) (Response, error) {
    var failed []BatchItemFailure
    
    for _, record := range event.Records {
        if err := processMessage(record); err != nil {
            failed = append(failed, BatchItemFailure{ItemIdentifier: record.MessageId})
        }
    }
    
    return Response{BatchItemFailures: failed}, nil
}
```

---

## S3 Trigger

Monitors a local directory for file changes and invokes your Lambda with S3-style event payloads.

### Configuration

```yaml
services:
  image-processor:
    image: public.ecr.aws/lambda/python:3.13
    codePath: ./services/image-processor
    cmd: ["main.handler"]
    triggers:
      - type: s3
        localPath: "./data/uploads"           # Directory to watch (required)
        bucket: "my-uploads-bucket"           # Logical bucket name (required)
        events:                                 # Event types to handle
          - "s3:ObjectCreated:*"
          - "s3:ObjectRemoved:*"
```

### Supported Event Types

| Event | Description |
|-------|-------------|
| `s3:ObjectCreated:*` | Any object creation |
| `s3:ObjectCreated:Put` | PUT object |
| `s3:ObjectCreated:Post` | POST object |
| `s3:ObjectCreated:Copy` | COPY object |
| `s3:ObjectRemoved:*` | Any object deletion |
| `s3:ObjectRemoved:Delete` | DELETE object |

### Event Format

```go
// Go handler
func Handler(ctx context.Context, event events.S3Event) error {
    for _, record := range event.Records {
        log.Printf("Bucket: %s, Key: %s, Event: %s",
            record.S3.Bucket.Name,
            record.S3.Object.Key,
            record.EventName)
    }
    return nil
}
```

```python
# Python handler
def handler(event, context):
    for record in event['Records']:
        bucket = record['s3']['bucket']['name']
        key = record['s3']['object']['key']
        print(f"Bucket: {bucket}, Key: {key}")
```

### Use Cases

- Image resizing/thumbnailing
- File format conversion
- Document processing
- Log aggregation
- Backup triggers

---

## SNS Trigger

Listens for messages published to an SNS topic and invokes your Lambda.

### Prerequisites

Requires a local SNS endpoint. For testing, you can use [SNS Local](https://github.com/j了新/moto) or publish directly.

### Configuration

```yaml
services:
  notifier:
    image: public.ecr.aws/lambda/python:3.13
    codePath: ./services/notifier
    cmd: ["main.handler"]
    triggers:
      - type: sns
        topicArn: "arn:aws:sns:local:000000000000:notifications"
        snsEndpointPort: 2772            # Optional (default: 2772)
```

### Publishing Messages

Publish to the local SNS endpoint:

```bash
curl -X POST http://localhost:2772/publish \
  -H "Content-Type: application/json" \
  -d '{
    "Message": "Hello from SNS",
    "Subject": "Test Notification",
    "TopicArn": "arn:aws:sns:local:000000000000:notifications"
  }'
```

### Event Format

```go
// Go handler
func Handler(ctx context.Context, event events.SNSEvent) error {
    for _, record := range event.Records {
        log.Printf("Topic: %s, Message: %s",
            record.SNS.TopicArn,
            record.SNS.Message)
    }
    return nil
}
```

```python
# Python handler
def handler(event, context):
    for record in event['Records']:
        topic = record['Sns']['TopicArn']
        message = record['Sns']['Message']
        print(f"Topic: {topic}, Message: {message}")
```

---

## DynamoDB Streams Trigger

Polls a DynamoDB Streams endpoint and invokes your Lambda with stream records.

### Prerequisites

Requires a DynamoDB Streams-enabled table. Use [DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html):

```bash
docker run -p 8000:8000 amazon/dynamodb-local
```

### Configuration

```yaml
services:
  audit-logger:
    image: public.ecr.aws/lambda/python:3.13
    codePath: ./services/audit-logger
    cmd: ["main.handler"]
    triggers:
      - type: dynamodb-stream
        streamArn: "arn:aws:dynamodb:local:000000000000:table/orders/stream/2024-01-01T00:00:00.000"
        dynamodbEndpoint: "http://localhost:8000"
        startingPosition: "LATEST"          # or "TRIM_HORIZON"
```

### Starting Position

| Value | Description |
|-------|-------------|
| `LATEST` | Process only new records |
| `TRIM_HORIZON` | Process all records from the beginning |

### Event Format

```go
// Go handler
func Handler(ctx context.Context, event events.DynamoDBEvent) error {
    for _, record := range event.Records {
        log.Printf("Table: %s, Operation: %s, ID: %s",
            record.EventSourceARN,
            record.EventName,
            recorddynamodbStreamRecord.EventID)
        
        for name, value := range record.Change.NewImage {
            log.Printf("Field: %s, Type: %s, Value: %v",
                name, value.DataType.String(), value)
        }
    }
    return nil
}
```

```python
# Python handler
def handler(event, context):
    for record in event['Records']:
        table_arn = record['eventSourceARN']
        operation = record['eventName']
        print(f"Table: {table_arn}, Operation: {operation}")
        
        if 'NewImage' in record['dynamodb']:
            print(f"New image: {record['dynamodb']['NewImage']}")
```

---

## Multiple Triggers

A single service can have multiple triggers:

```yaml
services:
  unified-processor:
    runtime: go
    codePath: ./services/processor
    cmd: ["main"]
    triggers:
      - type: schedule
        expression: "rate(1 hour)"
      
      - type: sqs
        queueUrl: "http://localhost:9324/queue/jobs"
      
      - type: s3
        localPath: "./data/input"
        bucket: "input-bucket"
```

Each trigger operates independently and invokes the same Lambda.

## Trigger Logging

Triggers log their activity. Use `simla logs <service-name>` to see trigger events:

```bash
simla logs unified-processor
```

Example output:
```
INFO[0000] sqs trigger started for service unified-processor (queue=http://localhost:9324/queue/jobs, batchSize=10)
INFO[0030] sqs: received 3 message(s)
INFO[0030] trigger invocation succeeded for service unified-processor
```

## Error Handling

If a Lambda invocation fails, the trigger logs the error and continues. For SQS triggers, failed messages are not deleted and will be retried on the next poll.

---

## Troubleshooting

### Trigger Not Starting

Check the configuration syntax and ensure the service is properly defined.

### SQS Messages Not Processed

- Verify queue URL is accessible
- Check Lambda handler doesn't error
- View logs: `simla logs <service-name>`

### S3 Events Not Triggering

- Ensure watched directory exists
- Check file permissions
- Verify event types match your operations

### SNS Messages Not Received

- Confirm SNS endpoint is running
- Check topic ARN matches configuration
- Verify messages are being published to the correct endpoint/port

## Best Practices

1. **Timeout Configuration**: Set appropriate timeouts for long-running tasks
2. **Error Handling**: Always handle errors in your Lambda
3. **Batch Size**: Use smaller batches for memory-intensive operations
4. **Logging**: Log sufficient information for debugging
5. **Idempotency**: Make your handlers idempotent (SQS may deliver duplicate messages)

## Next Steps

- [Getting Started](getting-started.md) - Quick setup guide
- [Configuration Reference](configuration.md) - All config options
- [Workflows](workflows.md) - Trigger workflows from events
