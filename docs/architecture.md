# Architecture Guide

This document describes the architecture of Simla and how its components work together.

## System Overview

Simla is a local serverless development platform that simulates AWS Lambda and AWS Step Functions. It uses Docker containers to run Lambda functions and implements an AWS Step Functions-compatible workflow engine.

```
┌─────────────────────────────────────────────────────────────────┐
│                          Simla CLI                               │
│            User Interface (Cobra Commands)                       │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Core Components                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Gateway    │  │  Scheduler   │  │  Workflow    │          │
│  │              │◄─┤              │◄─┤  Executor    │          │
│  │  HTTP Server │  │  Service     │  │              │          │
│  │              │  │  Lifecycle   │  │  State Machine│          │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘          │
│         │                 │                                      │
│         │                 ▼                                      │
│         │         ┌──────────────┐                               │
│         │         │  Registry    │                               │
│         │         │              │                               │
│         │         │  Port Alloca │                               │
│         │         │  Status Track│                               │
│         │         └──────┬───────┘                               │
│         │                │                                       │
│         │                ▼                                        │
│         │         ┌──────────────┐                               │
│         │         │   Runtime    │                               │
│         │         │              │                               │
│         │         │  Docker Mgmt │                               │
│         │         │  Container   │                               │
│         │         │  Lifecycle   │                               │
│         │         └──────┬───────┘                               │
│         │                │                                       │
└─────────┼────────────────┼───────────────────────────────────────┘
          │                │
          ▼                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Docker Containers                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Lambda #1   │  │  Lambda #2   │  │  Lambda #3   │          │
│  │  (payments)  │  │  (orders)    │  │  (notifier)  │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
│                   simla-network                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Components

### CLI (`cmd/simla/`)

The command-line interface built with [Cobra](https://github.com/spf13/cobra).

**Commands:**
- `up` - Start the server and all services
- `down` - Stop running services
- `invoke` - Directly invoke a service
- `logs` - Stream container logs
- `list` - List registered services
- `status` - Show invocation metrics
- `workflow` - Manage and run workflows

The CLI loads configuration from `.simla.yaml` using [Viper](https://github.com/spf13/viper).

### API Gateway (`internal/gateway/`)

HTTP server that routes incoming requests to Lambda services.

**Features:**
- HTTP routing based on configuration
- Request/Response transformation (APIGatewayV2 format)
- CORS support
- Request ID propagation
- 5-minute timeout for cold starts

**Endpoints:**
- `/<stage>/<path>` - Routes to configured services
- `/<stage>/health` - Health check endpoint

### Scheduler (`internal/scheduler/`)

Manages the lifecycle of Lambda services and routes invocations.

**Responsibilities:**
- Service registration
- Container startup/shutdown
- Health checking
- Request routing
- Metrics collection

**Flow:**
1. Receive invocation request
2. Check if service is registered and healthy
3. If not, start the container and wait for health
4. Forward request to container
5. Record metrics
6. Return response

### Service Registry (`internal/registry/`)

Persistent storage for service metadata.

**Stored Information:**
- Service name
- Allocated port
- Container ID
- Status (pending, running, stopped, failed)
- Health status

**Location:** `~/.simla/registry.yaml`

**Thread Safety:** Uses read-write mutex for concurrent access.

### Runtime (`internal/runtime/`)

Docker container management.

**Responsibilities:**
- Image pulling (with architecture verification)
- Container creation
- Network setup (simla-network)
- Port mapping
- Container lifecycle
- Log streaming

**Architecture Support:**
- `amd64` (x86_64)
- `arm64` (aarch64)

Cross-architecture warnings are logged if emulation would be required.

### Workflow Executor (`internal/workflow/`)

AWS Step Functions-compatible state machine engine.

**Supported States:**
- Task - Invoke Lambda service
- Pass - Data transformation
- Choice - Conditional branching
- Parallel - Concurrent branches
- Wait - Pause execution
- Succeed - Terminal success
- Fail - Terminal failure

**Features:**
- InputPath/OutputPath/ResultPath data flow
- Retry with exponential backoff and jitter
- Catch error handling
- Parallel branch execution (goroutines)

### Triggers (`internal/trigger/`)

Event source implementations.

| Trigger | Package | Description |
|---------|---------|-------------|
| Schedule | `schedule.go` | Time-based (rate/cron) |
| SQS | `sqs.go` | Queue polling |
| S3 | `s3.go` | File system watcher |
| SNS | `sns.go` | HTTP endpoint listener |
| DynamoDB | `dynamodb.go` | Stream polling |

All triggers run as background goroutines when `simla up` is executed.

### Health Checker (`internal/health/`)

Monitors Lambda service health.

**Check Method:**
- HTTP GET to Lambda invocation endpoint
- 5-second timeout
- 30-second wait timeout

**Health Endpoint:** `http://localhost:<port>/2015-03-31/functions/function/invocations`

### Metrics (`internal/metrics/`)

Invocation statistics collection.

**Tracked Metrics:**
- Invocation count
- Error count
- Total latency
- Last invocation timestamp

**Access:** `simla status` command

### Watcher (`internal/watcher/`)

File system monitoring for hot reload.

**Behavior:**
- Watches service `codePath` directories
- Debounces events (500ms default)
- Triggers service restart on changes

**Requirements:** `--watch` flag with `simla up`

## Data Flow

### HTTP Request Flow

```
HTTP Request
    │
    ▼
API Gateway (HTTP routing)
    │
    ▼
Scheduler.Invoke()
    │
    ├──► Registry: Get service info
    │
    ├──► Runtime: Start container (if needed)
    │
    ├──► Health Check: Wait for healthy
    │
    ▼
Router.SendRequest()
    │
    ▼
Docker Container (Lambda)
    │
    ▼
Response
```

### Workflow Execution Flow

```
simla workflow run <name> --input <json>
    │
    ▼
Workflow Executor.Execute()
    │
    ▼
For each state:
    │
    ├──► Execute state type handler
    │
    ├──► Apply InputPath/ResultPath/OutputPath
    │
    ├──► Check for errors → Retry/Catch
    │
    ├──► Determine next state
    │
    ▼
Return final output
```

### Trigger Event Flow

```
Trigger.Start()
    │
    ├──► Loop:
    │       │
    │       ▼
    │   Poll/Listen for events
    │       │
    │       ▼
    │   Build Lambda event payload
    │       │
    │       ▼
    │   Scheduler.Invoke()
    │       │
    │       ▼
    │   Lambda executes
    │       │
    ▼
Context cancelled → Exit
```

## Configuration Loading

```
.simla.yaml
    │
    ▼
Viper (reads & unmarshals)
    │
    ▼
config.Config struct
    │
    ├──► Services → Service Registry
    │
    ├──► Routes → API Gateway
    │
    ├──► Triggers → Trigger initialization
    │
    └──► Workflows → Workflow Executor
```

## Directory Structure

```
simla/
├── cmd/
│   ├── main.go                    # Entry point
│   └── simla/                    # CLI commands
│       ├── root.go               # Root command, config loading
│       ├── up.go                 # Start server
│       ├── down.go               # Stop services
│       ├── invoke.go             # Direct invoke
│       ├── logs.go               # Container logs
│       ├── list.go               # List services
│       ├── status.go             # Metrics display
│       └── workflow.go           # Workflow commands
│
├── internal/
│   ├── config/                   # Configuration types
│   │   ├── config.go             # Config struct, Viper unmarshal
│   │   ├── types.go              # Service, Route, CORS types
│   │   └── workflow.go           # State machine types
│   │
│   ├── gateway/                  # HTTP API Gateway
│   │   ├── gateway.go            # Main server, routing
│   │   └── types.go              # Gateway interfaces
│   │
│   ├── scheduler/                # Service scheduler
│   │   ├── scheduler.go          # Service lifecycle
│   │   ├── router.go             # HTTP client
│   │   └── types.go              # Scheduler interfaces
│   │
│   ├── registry/                 # Service registry
│   │   ├── registry.go           # In-memory + YAML persistence
│   │   └── types.go              # Registry types
│   │
│   ├── runtime/                  # Docker runtime
│   │   ├── runtime.go            # Container management
│   │   └── types.go              # Runtime interfaces
│   │
│   ├── workflow/                 # Workflow executor
│   │   ├── executor.go           # State machine logic
│   │   ├── jsonpath.go           # JSONPath implementation
│   │   └── types.go              # Workflow types
│   │
│   ├── trigger/                  # Event triggers
│   │   ├── types.go              # Trigger interface
│   │   ├── schedule.go           # CloudWatch Events
│   │   ├── sqs.go                # SQS polling
│   │   ├── s3.go                 # File system watcher
│   │   ├── sns.go                # SNS listener
│   │   └── dynamodb.go           # DynamoDB Streams
│   │
│   ├── health/                   # Health checking
│   │   └── health.go             # HTTP health checks
│   │
│   ├── metrics/                  # Invocation metrics
│   │   └── metrics.go            # Statistics collection
│   │
│   ├── watcher/                 # Hot reload
│   │   └── watcher.go           # File system watcher
│   │
│   └── errors/                  # Error types
│       └── errors.go            # Custom error types
│
├── docs/                        # Documentation
├── examples/                    # Example Lambda functions
├── Makefile                     # Build commands
└── README.md                    # This file
```

## Concurrency Model

- **HTTP Gateway**: Handles each request in its own goroutine
- **Triggers**: Each trigger runs in its own goroutine
- **Workflow Parallel**: Each branch runs in a goroutine (`sync.WaitGroup`)
- **Health Checks**: Per-service polling loops
- **Watcher**: Single fsnotify watcher, debounced restarts

## Error Handling

### Service Invocation Errors
- Retried based on Retry configuration
- Errors propagate to workflow Catch handlers
- Metrics track error rates

### Container Errors
- Logs are streamed for debugging
- Service status updated to "failed"
- Registry persists state

### Workflow Errors
- Retry with exponential backoff (optional)
- Catch blocks handle specific errors
- Unhandled errors fail the workflow

## Security Considerations

> **Note:** See `recommendations.md` for known security considerations and recommendations.

Current security measures:
- Environment variables are masked in logs (SECRET, PASSWORD, TOKEN, KEY)
- Container network isolation via Docker
- Context propagation for request tracing

## Extension Points

### Adding a New Runtime

1. Add runtime constant in `runtime.go`
2. Implement `inferImageFromRuntime()` function
3. Update examples and documentation

### Adding a New Trigger

1. Create new file in `internal/trigger/`
2. Implement `Source` interface
3. Register in `trigger.New()`
4. Add type constant in `config/types.go`

### Adding a New State Type

1. Add type constant in `config/workflow.go`
2. Add case in `executeState()` switch
3. Implement state handler function
4. Add tests

## Performance Considerations

- **Container Reuse**: Containers stay running between invocations
- **Health Check Caching**: Recent healthy checks skip redundant checks
- **Image Caching**: Architecture-verified image caching
- **Debounced Watching**: File change events are debounced

## Next Steps

- [Getting Started](getting-started.md) - Build your first service
- [Configuration](configuration.md) - All configuration options
- [Workflows](workflows.md) - State machine patterns
- [Triggers](triggers.md) - Event-driven invocations
