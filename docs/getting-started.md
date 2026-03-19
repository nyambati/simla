# Getting Started with Simla

This guide will walk you through setting up Simla and creating your first serverless application.

## Prerequisites

Before you begin, ensure you have:

- **Go 1.23+** - [Install Go](https://golang.org/doc/install)
- **Docker** - [Install Docker](https://docs.docker.com/get-docker/)
- A terminal/command prompt

## Installation

### 1. Clone and Build

```bash
git clone https://github.com/nyambati/simla.git
cd simla
make build
```

This creates the `simla` binary in the `bin/` directory.

### 2. Add to PATH (Optional)

Add the binary to your PATH for convenience:

```bash
# For bash
echo 'export PATH=$PATH:$(pwd)/bin' >> ~/.bashrc
source ~/.bashrc

# For zsh
echo 'export PATH=$PATH:$(pwd)/bin' >> ~/.zshrc
source ~/.zshrc
```

### 3. Verify Installation

```bash
simla --help
```

You should see the Simla help output.

## Creating Your First Service

### Step 1: Create Project Structure

```bash
mkdir my-project && cd my-project
mkdir -p hello
```

### Step 2: Create Configuration File

Create a `.simla.yaml` file in your project root:

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

### Step 3: Write Lambda Handler

Create `hello/main.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/aws/aws-lambda-go/lambda"
)

// Response implements the APIGatewayV2HTTPResponse format
type Response struct {
    StatusCode int               `json:"statusCode"`
    Body       string            `json:"body"`
    Headers    map[string]string `json:"headers"`
}

type Request struct {
    QueryStringParameters map[string]string `json:"queryStringParameters"`
}

func Handler(ctx context.Context, event Request) (Response, error) {
    name := "World"
    if nameParam, ok := event.QueryStringParameters["name"]; ok {
        name = nameParam
    }

    return Response{
        StatusCode: 200,
        Body:       fmt.Sprintf(`{"message":"Hello, %s!"}`, name),
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
    }, nil
}

func main() {
    lambda.Start(Handler)
}
```

### Step 4: Build the Lambda

Simla runs Lambda functions inside Docker containers, so you must build for Linux:

```bash
GOOS=linux GOARCH=amd64 go build -o hello/main hello/main.go
```

### Step 5: Start Simla

```bash
simla up
```

You'll see output like:

```
INFO[0000] starting gateway on port 8080
INFO[0000] registering route method=GET path=/v1/hello service=hello-service
INFO[0000] starting gateway on port 8080
```

### Step 6: Test Your Service

In another terminal:

```bash
curl http://localhost:8080/v1/hello
# {"message":"Hello, World!"}

curl http://localhost:8080/v1/hello?name=Alice
# {"message":"Hello, Alice!"}
```

## Using Hot Reload

When developing, use the `--watch` flag to automatically restart services when code changes:

```bash
simla up --watch
```

Now when you modify your Lambda code:

1. Rebuild: `GOOS=linux GOARCH=amd64 go build -o hello/main hello/main.go`
2. Simla automatically detects the change and restarts the container

## Creating a Python Lambda

Simla also supports Python Lambda functions.

### Step 1: Create Python Handler

Create `python-hello/main.py`:

```python
import json

def handler(event, context):
    name = event.get('queryStringParameters', {}).get('name', 'World')
    
    return {
        'statusCode': 200,
        'body': json.dumps({'message': f'Hello, {name}!'}),
        'headers': {
            'Content-Type': 'application/json'
        }
    }
```

### Step 2: Update Configuration

```yaml
apiGateway:
  port: 8080
  stage: v1
  routes:
    - path: hello
      method: GET
      service: python-hello

services:
  python-hello:
    image: public.ecr.aws/lambda/python:3.13
    codePath: ./python-hello
    cmd: ["main.handler"]
```

### Step 3: Test

```bash
simla up
curl http://localhost:8080/v1/hello?name=PythonDev
```

## Working with Multiple Services

Simla can manage multiple Lambda services simultaneously.

### Configuration

```yaml
apiGateway:
  port: 8080
  stage: v1
  routes:
    - path: users
      method: GET
      service: user-service
    - path: users
      method: POST
      service: user-service
    - path: orders
      method: GET
      service: order-service

services:
  user-service:
    runtime: go
    codePath: ./services/user
    cmd: ["main"]

  order-service:
    runtime: go
    codePath: ./services/order
    cmd: ["main"]
```

## Using Environment Variables

### Inline Environment Variables

```yaml
services:
  my-service:
    runtime: go
    codePath: ./service
    cmd: ["main"]
    environment:
      DATABASE_URL: "postgres://localhost:5432/mydb"
      API_KEY: "secret-key"
      LOG_LEVEL: "debug"
```

### External .env File

Create a `.env` file:

```bash
DATABASE_URL=postgres://localhost:5432/mydb
API_KEY=secret-key
EXTERNAL_SERVICE_URL=${EXTERNAL_API_URL:-https://default.example.com}
```

Reference it in your config:

```yaml
services:
  my-service:
    runtime: go
    codePath: ./service
    cmd: ["main"]
    envFile: .env
```

Environment variable interpolation supports:
- `${VAR}` - Replace with environment variable value
- `${VAR:-default}` - Use default if VAR is not set

## Managing Services

### List Running Services

```bash
simla list
```

### Invoke a Service Directly

```bash
simla invoke my-service --payload '{"key":"value"}'
```

### View Service Logs

```bash
# View recent logs
simla logs my-service

# Follow logs in real-time
simla logs my-service --follow
```

### Stop Services

```bash
# Stop a specific service
simla down my-service

# Stop all services
simla down
```

## Service Health and Metrics

### Check Service Status

```bash
simla status
```

This shows invocation counts, error rates, and latency metrics.

## Environment Variables Reference

Simla recognizes the following environment variables:

| Variable | Description |
|----------|-------------|
| `HOME` | User home directory (for registry storage) |

## Troubleshooting

### Service Won't Start

Check if the port is already in use:
```bash
lsof -i :8080
```

### Container Fails to Start

View logs for details:
```bash
simla logs <service-name>
```

### Build Errors

Ensure you're building for Linux:
```bash
GOOS=linux GOARCH=amd64 go build -o output_path source.go
```

## Next Steps

- [Configuration Reference](configuration.md) - All configuration options
- [Workflows](workflows.md) - Orchestrate multiple Lambda functions
- [Triggers](triggers.md) - Event-driven invocations
- [Architecture](architecture.md) - Understand how Simla works
