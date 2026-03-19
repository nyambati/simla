# Configuration Reference

This document provides a complete reference for all configuration options in Simla's `.simla.yaml` file.

## Configuration File Location

Simla looks for `.simla.yaml` in the current working directory. You can also specify a custom path:

```bash
simla --config /path/to/config.yaml up
```

## Top-Level Structure

```yaml
apiGateway:          # API Gateway configuration
  port: "8080"
  stage: "v1"
  cors: {...}

services:            # Lambda service definitions
  service-name:
    ...

workflows:           # Step Functions workflow definitions
  workflow-name:
    ...
```

---

## API Gateway Configuration

```yaml
apiGateway:
  port: "8080"           # Port for HTTP server (required)
  stage: "v1"           # API stage prefix (default: "v1")
  cors: {...}            # CORS settings (optional)
  routes: [...]          # HTTP route definitions
```

### CORS Configuration

```yaml
cors:
  enabled: true                    # Enable CORS headers
  allowOrigins: "*"                # Allowed origins (required if enabled)
  allowMethods: "GET,POST,PUT,DELETE,OPTIONS"
  allowHeaders: "Content-Type,Authorization,X-Request-ID"
  allowCredentials: false          # Allow credentials (cannot be true with "*")
  maxAge: 86400                   # Preflight cache duration in seconds
```

**Important:** When `allowCredentials` is `true`, `allowOrigins` cannot be `"*"`.

### Routes

```yaml
routes:
  - path: "/users"              # URL path (required)
    method: "GET"               # HTTP method (required)
    service: "user-service"     # Service name (required)
```

Routes are prefixed with the `stage` value (e.g., `/v1/users`).

---

## Service Configuration

```yaml
services:
  service-name:
    # Runtime Selection
    runtime: "go"               # Lambda runtime name
    # OR
    image: "public.ecr.aws/lambda/python:3.13"  # Custom Docker image
    
    # Deployment
    architecture: "amd64"        # amd64 or arm64 (auto-detected if omitted)
    codePath: "./path/to/code"   # Local path to Lambda code
    cmd: ["handler"]            # Command to execute
    entrypoint: []              # Container entrypoint (optional)
    
    # Environment
    environment:                # Inline environment variables
      KEY: "value"
    envFile: ".env"             # Path to .env file
    
    # Triggers
    triggers:
      - type: "schedule"
        expression: "rate(5 minutes)"
```

### Runtimes

Simla supports standard AWS Lambda runtimes:

| Runtime | Image |
|---------|-------|
| `go` | `public.ecr.aws/lambda/go:1` |
| `python` | `public.ecr.aws/lambda/python:3.13` |
| `python3.12` | `public.ecr.aws/lambda/python:3.12` |
| `python3.11` | `public.ecr.aws/lambda/python:3.11` |
| `python3.10` | `public.ecr.aws/lambda/python:3.10` |
| `nodejs20.x` | `public.ecr.aws/lambda/nodejs:20` |
| `nodejs18.x` | `public.ecr.aws/lambda/nodejs:18` |
| `nodejs16.x` | `public.ecr.aws/lambda/nodejs:16` |

### Service Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `runtime` | string | Yes* | Lambda runtime name |
| `image` | string | Yes* | Custom Docker image |
| `architecture` | string | No | `amd64` or `arm64` |
| `codePath` | string | Yes | Path to Lambda code |
| `cmd` | []string | No | Command arguments |
| `entrypoint` | []string | No | Container entrypoint |
| `environment` | map | No | Environment variables |
| `envFile` | string | No | Path to .env file |
| `triggers` | []Trigger | No | Event triggers |

*Either `runtime` or `image` must be specified.

### Environment Variable Interpolation

Environment variables support `${VAR}` and `${VAR:-default}` syntax:

```yaml
environment:
  DB_HOST: "${DB_HOST:-localhost}"
  API_KEY: "${API_KEY}"
  CONFIG_PATH: "${HOME}/.config/myapp"
```

---

## Trigger Configuration

Triggers invoke Lambda services based on external events. See [Triggers Guide](triggers.md) for detailed documentation.

### Schedule Trigger

```yaml
triggers:
  - type: schedule
    expression: "rate(5 minutes)"     # Rate expression
    # OR
    expression: "cron(0 12 * * ? *)"  # Cron expression
```

### SQS Trigger

```yaml
triggers:
  - type: sqs
    queueUrl: "http://localhost:9324/queue/my-queue"
    batchSize: 10                     # Max messages per batch (default: 10)
    pollingInterval: "2s"             # Polling frequency (default: 1s)
```

### S3 Trigger

```yaml
triggers:
  - type: s3
    localPath: "./data/uploads"
    bucket: "my-uploads-bucket"
    events:
      - "s3:ObjectCreated:*"
      - "s3:ObjectRemoved:*"
```

### SNS Trigger

```yaml
triggers:
  - type: sns
    topicArn: "arn:aws:sns:local:000000000000:notifications"
    snsEndpointPort: 2772            # Optional (default: 2772)
```

### DynamoDB Streams Trigger

```yaml
triggers:
  - type: dynamodb-stream
    streamArn: "arn:aws:dynamodb:local:000000000000:table/orders/stream/2024-01-01T00:00:00.000"
    dynamodbEndpoint: "http://localhost:8000"
    startingPosition: "LATEST"       # or "TRIM_HORIZON"
```

---

## Workflow Configuration

Workflows define state machines that orchestrate Lambda invocations. See [Workflows Guide](workflows.md) for detailed documentation.

### Basic Structure

```yaml
workflows:
  workflow-name:
    comment: "Optional description"
    startAt: "first-state"
    states:
      first-state:
        Type: "Task"
        Resource: "service-name"
        next: "second-state"
      second-state:
        Type: "Pass"
        Result:
          status: "complete"
        End: true
```

### State Types

| Type | Description |
|------|-------------|
| `Task` | Invoke a Lambda service |
| `Pass` | Transform data without service call |
| `Choice` | Conditional branching |
| `Parallel` | Execute branches concurrently |
| `Wait` | Pause execution |
| `Succeed` | End successfully |
| `Fail` | End with error |

### Task State Properties

```yaml
TaskState:
  Type: "Task"
  Resource: "service-name"          # Service to invoke
  Next: "next-state"                # Next state (if not End)
  End: false                        # End workflow here
  TimeoutSeconds: 60                # Task timeout
  Retry: [...]                      # Retry configuration
  Catch: [...]                      # Error handling
  InputPath: "$.input"              # Extract from input
  OutputPath: "$.output"           # Filter output
  ResultPath: "$.result"           # Merge result
```

### Retry Configuration

```yaml
Retry:
  - ErrorEquals: ["States.Timeout", "States.TaskFailed"]
    IntervalSeconds: 2
    MaxAttempts: 3
    BackoffRate: 2.0
    Jitter: true
```

### Catch Configuration

```yaml
Catch:
  - ErrorEquals: ["States.ALL"]
    Next: "fallback-state"
    ResultPath: "$.error"
```

---

## Complete Example

```yaml
apiGateway:
  port: 8080
  stage: v1
  cors:
    enabled: true
    allowOrigins: "*"
    allowMethods: "GET,POST,PUT,DELETE,OPTIONS"
    allowHeaders: "Content-Type,Authorization,X-Request-ID"
  routes:
    - path: orders
      method: GET
      service: order-service
    - path: orders
      method: POST
      service: order-service

services:
  order-service:
    runtime: go
    architecture: amd64
    codePath: ./services/orders
    cmd: ["main"]
    environment:
      NODE_ENV: development
      TIMEOUT: "30"
    triggers:
      - type: sqs
        queueUrl: "http://localhost:9324/queue/orders"
        batchSize: 5

  payment-service:
    runtime: go
    codePath: ./services/payments
    cmd: ["main"]
    envFile: .env

workflows:
  order-processor:
    startAt: validate
    states:
      validate:
        Type: Choice
        choices:
          - variable: "$.order.total"
            numericGreaterThan: 0
            next: process
        default: reject
      process:
        Type: Parallel
        branches:
          - startAt: payment
            states:
              payment:
                Type: Task
                Resource: payment-service
                End: true
        resultPath: "$.results"
        next: confirm
      confirm:
        Type: Task
        Resource: order-service
        End: true
      reject:
        Type: Fail
        Error: InvalidOrder
        Cause: Order total must be greater than zero
```

---

## Configuration Validation

Simla validates configuration on startup:

- Services must have either `runtime` or `image`
- `codePath` must be a valid directory
- Workflows must have `startAt` and `states`
- Route `service` values must match defined services

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `HOME` | User home directory (registry storage: `~/.simla/registry.yaml`) |

---

## Next Steps

- [Getting Started](getting-started.md) - Quick setup guide
- [Workflows](workflows.md) - State machine patterns
- [Triggers](triggers.md) - Event source configuration
- [Architecture](architecture.md) - System design
