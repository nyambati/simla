# Simla

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Simla is a lightweight, extensible, and open-source serverless framework that enables developers to build, test, and deploy serverless applications locally. It provides a local environment that simulates the behavior of AWS Lambda and AWS Step Functions, allowing developers to iterate quickly and debug their applications with ease.

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [CLI Reference](#cli-reference)
- [Configuration](#configuration)
- [Services](#services)
- [Workflows](#workflows)
- [Triggers](#triggers)
- [Architecture](#architecture)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Local Lambda Simulation**: Run AWS Lambda functions locally without deploying to the cloud
- **Workflow Engine**: Execute AWS Step Functions-style state machines locally
- **Multi-Language Support**: Support for Go, Python, and any Lambda-compatible runtime via custom Docker images
- **Built-in API Gateway**: HTTP endpoints that map to Lambda functions
- **Docker Integration**: Containerized execution ensures consistent behavior across environments
- **Hot Reload**: Automatic container restart when code changes (with `--watch` flag)
- **Event Triggers**: Schedule, SQS, S3, SNS, and DynamoDB Streams event sources
- **Service Registry**: Persistent tracking of running services with health monitoring
- **Invocation Metrics**: Per-service latency and error rate tracking

## Quick Start

### Prerequisites

- [Go](https://golang.org/doc/install) 1.23 or later
- [Docker](https://docs.docker.com/get-docker/)

### Build and Run

```bash
# Clone the repository
git clone https://github.com/nyambati/simla.git
cd simla

# Build the CLI
make build

# Add to PATH (optional)
export PATH=$PATH:$(pwd)/bin

# Start the server
simla up
```

### Your First Service

1. Create a `.simla.yaml` configuration file:

```yaml
apiGateway:
  port: 8080
  stage: v1
  routes:
    - path: hello
      method: GET
      service: hello-service

services:
  hello-service:
    runtime: go
    codePath: ./hello
    cmd: ["main"]
    environment:
      NODE_ENV: development
```

2. Create a Lambda handler in `hello/main.go`:

```go
package main

import (
    "context"
    "fmt"
    "github.com/aws/aws-lambda-go/lambda"
)

func Handler(ctx context.Context, name string) (string, error) {
    return fmt.Sprintf("Hello, %s!", name), nil
}

func main() {
    lambda.Start(Handler)
}
```

3. Build the Lambda (for Linux):

```bash
GOOS=linux GOARCH=amd64 go build -o hello/main hello/main.go
```

4. Start Simla and test:

```bash
simla up --watch
```

In another terminal:

```bash
curl http://localhost:8080/v1/hello?name=World
# Output: Hello, World!
```

## Installation

### From Source

```bash
git clone https://github.com/nyambati/simla.git
cd simla
make build
```

The binary will be created at `bin/simla`.

### Add to PATH

```bash
# Add to ~/.bashrc or ~/.zshrc
export PATH=$PATH:/path/to/simla/bin
```

### Verify Installation

```bash
simla --version
```

## CLI Reference

### `simla up`

Start the Simla server with all configured services.

```bash
simla up [flags]
```

**Flags:**
- `-w, --watch`: Enable hot reload - restart services when code changes

**Example:**
```bash
simla up --watch
```

### `simla down`

Stop running Lambda containers.

```bash
simla down [service-name]
```

**Examples:**
```bash
simla down           # Stop all services
simla down payments  # Stop only the payments service
```

### `simla invoke`

Directly invoke a Lambda service (bypasses HTTP gateway).

```bash
simla invoke <service-name> [flags]
```

**Flags:**
- `-p, --payload`: JSON payload to send
- `-f, --file`: Path to JSON file for payload

**Example:**
```bash
simla invoke payments --payload '{"name":"Alice"}'
```

### `simla logs`

Stream logs from a service container.

```bash
simla logs <service-name> [flags]
```

**Flags:**
- `-f, --follow`: Follow log output (stream until container stops)

**Example:**
```bash
simla logs payments --follow
```

### `simla list`

List all registered services with their status.

```bash
simla list
```

**Output:**
```
NAME            STATUS      PORT    CONTAINER ID    HEALTHY
----            ------      ----    ------------    -------
payments        running     9001    a1b2c3d4e5f6    true
order-processor pending     9002    -               false
```

### `simla status`

Show per-service invocation metrics.

```bash
simla status
```

**Output:**
```
SERVICE         INVOCATIONS    ERRORS    ERROR RATE    AVG LATENCY    LAST INVOKED
-------         -----------    ------    ----------    -----------    ------------
payments        150            3         0.02          45ms           2025-01-15T10:30:00Z
```

### `simla workflow`

Manage and execute Step Functions workflows.

#### List Workflows

```bash
simla workflow list
```

#### Run a Workflow

```bash
simla workflow run <workflow-name> [flags]
```

**Flags:**
- `-i, --input`: JSON input string
- `-f, --file`: Path to JSON input file
- `--pretty`: Pretty-print JSON output

**Example:**
```bash
simla workflow run order-pipeline --input '{"orderId":"123"}' --pretty
```

## Configuration

Simla uses a `.simla.yaml` file in your project root for configuration. See [Configuration Guide](docs/configuration.md) for detailed documentation.

### Basic Structure

```yaml
apiGateway:
  port: 8080
  stage: v1
  cors:
    enabled: true
    allowOrigins: "*"
    allowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS"
    allowHeaders: "Content-Type,Authorization,X-Request-ID"
    maxAge: 86400
  routes:
    - path: payments
      method: GET
      service: payments

services:
  service-name:
    runtime: go          # or python, nodejs16x, etc.
    image: custom/image  # or use runtime
    architecture: amd64  # or arm64
    codePath: ./path
    cmd: ["handler"]
    entrypoint: []
    environment:
      KEY: value
    envFile: .env       # optional .env file
    triggers:
      - type: sqs
        queueUrl: "http://localhost:9324/queue/my-queue"

workflows:
  workflow-name:
    startAt: state-name
    states:
      state-name:
        Type: Task
        Resource: service-name
```

## Services

Services are Lambda functions that can be invoked via HTTP or triggers. See the [Getting Started Guide](docs/getting-started.md) for detailed examples.

### Service Properties

| Property | Type | Description |
|----------|------|-------------|
| `runtime` | string | Lambda runtime (go, python, nodejs, etc.) |
| `image` | string | Custom Docker image (overrides runtime) |
| `architecture` | string | amd64 or arm64 |
| `codePath` | string | Path to Lambda code |
| `cmd` | []string | Command to execute |
| `entrypoint` | []string | Container entrypoint |
| `environment` | map | Environment variables |
| `envFile` | string | Path to .env file |
| `triggers` | []Trigger | Event source triggers |

### Lambda Handler Examples

#### Go

```go
package main

import (
    "context"
    "github.com/aws/aws-lambda-go/lambda"
)

func Handler(ctx context.Context, event struct {
    Name string `json:"name"`
}) (string, error) {
    return "Hello, " + event.Name + "!", nil
}

func main() {
    lambda.Start(Handler)
}
```

#### Python

```python
import json

def handler(event, context):
    name = event.get('name', 'World')
    return f"Hello, {name}!"
```

### Building Lambdas for Simla

Simla runs Lambdas inside Docker containers. Build your functions for Linux:

```bash
# Go
GOOS=linux GOARCH=amd64 go build -o output_path main.go

# Python (typically already compatible)
```

## Workflows

Simla implements an AWS Step Functions-compatible workflow engine. Define state machines that orchestrate multiple Lambda invocations with support for parallel execution, choices, retries, and error handling. See the [Workflow Guide](docs/workflows.md) for detailed documentation.

### Supported State Types

- **Task**: Invoke a Lambda service
- **Pass**: Transform data without invoking a service
- **Choice**: Conditional branching
- **Parallel**: Execute multiple branches concurrently
- **Wait**: Pause execution for a duration or until a timestamp
- **Succeed**: End workflow successfully
- **Fail**: End workflow with an error

### Example Workflow

```yaml
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
          - startAt: charge
            states:
              charge:
                Type: Task
                resource: payment-service
                end: true
          - startAt: fulfill
            states:
              fulfill:
                Type: Task
                resource: fulfillment-service
                end: true
        resultPath: "$.results"
        next: confirm
      confirm:
        Type: Task
        resource: notification-service
        end: true
      reject:
        Type: Fail
        error: InvalidOrder
        cause: Order total must be greater than zero
```

### Data Flow

Workflows pass JSON data between states using:

- **InputPath**: Select portion of input to pass to state
- **OutputPath**: Filter output before passing to next state
- **ResultPath**: Merge task result into the input document

```yaml
states:
  example:
    Type: Task
    Resource: my-service
    inputPath: "$.payload"      # Extract from input
    resultPath: "$.serviceResult" # Merge result
    outputPath: "$.serviceResult" # Filter output
    next: next-state
```

## Triggers

Triggers invoke Lambda services based on external events. See the [Triggers Guide](docs/triggers.md) for detailed documentation.

### Supported Triggers

#### Schedule (CloudWatch Events)

```yaml
triggers:
  - type: schedule
    expression: "rate(5 minutes)"
    # or cron: "cron(0 12 * * ? *)"
```

#### SQS (Simple Queue Service)

```yaml
triggers:
  - type: sqs
    queueUrl: "http://localhost:9324/queue/orders"
    batchSize: 10
    pollingInterval: "2s"
```

#### S3 (File System Events)

```yaml
triggers:
  - type: s3
    localPath: ./data/uploads
    bucket: my-uploads-bucket
    events:
      - "s3:ObjectCreated:*"
      - "s3:ObjectRemoved:*"
```

#### SNS (Simple Notification Service)

```yaml
triggers:
  - type: sns
    topicArn: "arn:aws:sns:local:000000000000:notifications"
    snsEndpointPort: 2772
```

#### DynamoDB Streams

```yaml
triggers:
  - type: dynamodb-stream
    streamArn: "arn:aws:dynamodb:local:000000000000:table/orders/stream/2024-01-01T00:00:00.000"
    dynamodbEndpoint: "http://localhost:8000"
    startingPosition: LATEST
```

## Architecture

Simla's architecture consists of several interconnected components. See the [Architecture Guide](docs/architecture.md) for detailed documentation.

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI                                  │
│  (Cobra commands: up, down, invoke, logs, workflow, etc.)   │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│                    API Gateway                               │
│  - HTTP routing (gorilla/mux)                               │
│  - CORS handling                                            │
│  - Request/Response transformation                           │
└────────┬───────────────────────────────────┬────────────────┘
         │                                   │
┌────────▼────────┐               ┌───────────▼────────────────┐
│    Triggers     │               │       Scheduler           │
│  - Schedule     │               │  - Service lifecycle      │
│  - SQS          │               │  - Health checks         │
│  - S3           │               │  - Request routing       │
│  - SNS          │               └───────────┬────────────────┘
│  - DynamoDB     │                           │
└─────────────────┘                           │
                                    ┌─────────▼────────────────┐
                                    │    Service Registry      │
                                    │  - Service discovery     │
                                    │  - Port allocation       │
                                    │  - Status tracking       │
                                    └─────────┬────────────────┘
                                              │
                                    ┌─────────▼────────────────┐
                                    │       Runtime           │
                                    │  - Docker container mgmt │
                                    │  - Image pulling        │
                                    │  - Port mapping         │
                                    └─────────────────────────┘
                                              │
                                    ┌─────────▼────────────────┐
                                    │   Docker Containers     │
                                    │  (Lambda Functions)     │
                                    └─────────────────────────┘
```

### Components

- **CLI**: Command-line interface built with Cobra
- **API Gateway**: HTTP server that routes requests to Lambda services
- **Scheduler**: Manages service lifecycle and request routing
- **Service Registry**: Tracks running services, ports, and health status
- **Runtime**: Docker container management for Lambda execution
- **Workflow Executor**: AWS Step Functions-compatible state machine engine
- **Triggers**: Event source handlers for various AWS services

## Examples

The `examples/` directory contains complete working examples:

| Example | Language | Description |
|---------|----------|-------------|
| `go/main.go` | Go | Basic HTTP Lambda handler |
| `go/schedule/main.go` | Go | CloudWatch scheduled Lambda |
| `go/sqs/main.go` | Go | SQS batch processor |
| `python/main.py` | Python | Basic Python Lambda |
| `python/s3/main.py` | Python | S3 event handler |
| `python/sns/main.py` | Python | SNS message handler |
| `python/dynamodb/main.py` | Python | DynamoDB stream handler |

### Running Examples

```bash
# Build all Go examples
cd examples
GOOS=linux GOARCH=amd64 go build -o bin/payments go/main.go
GOOS=linux GOARCH=amd64 go build -o bin/reconciler go/schedule/main.go
GOOS=linux GOARCH=amd64 go build -o bin/order-processor go/sqs/main.go

# Start simla with example config
simla up --watch
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
make ptest
```

### Project Structure

```
simla/
├── cmd/
│   ├── main.go              # Entry point
│   └── simla/               # CLI commands
│       ├── root.go          # Root command
│       ├── up.go            # Start server
│       ├── down.go          # Stop services
│       ├── invoke.go        # Direct invoke
│       ├── logs.go          # Container logs
│       ├── list.go          # List services
│       ├── status.go        # Metrics
│       └── workflow.go      # Workflow commands
├── internal/
│   ├── config/              # Configuration types
│   ├── gateway/             # HTTP API gateway
│   ├── scheduler/           # Service scheduler
│   ├── registry/            # Service registry
│   ├── runtime/             # Docker runtime
│   ├── workflow/            # Workflow executor
│   ├── trigger/             # Event triggers
│   ├── health/              # Health checking
│   ├── metrics/             # Invocation metrics
│   ├── env/                 # Environment resolution
│   ├── watcher/             # File system watcher
│   └── errors/             # Error types
├── examples/                # Example Lambda functions
├── docs/                    # Documentation
├── recommendations.md       # Security review findings
├── Makefile                 # Build commands
└── README.md                # This file
```

## Documentation

For detailed documentation, see:

- [Getting Started](docs/getting-started.md) - Complete setup and first service
- [Configuration Reference](docs/configuration.md) - All configuration options
- [Workflow Guide](docs/workflows.md) - State machine examples and patterns
- [Triggers Guide](docs/triggers.md) - Event source configuration
- [Architecture](docs/architecture.md) - System design and components

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

Simla is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Links

- [Documentation](docs/)
- [GitHub Repository](https://github.com/nyambati/simla)
- [Issue Tracker](https://github.com/nyambati/simla/issues)
