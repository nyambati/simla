# Workflows Guide

Simla implements an AWS Step Functions-compatible workflow engine that allows you to orchestrate multiple Lambda services into state machines.

## Overview

Workflows are defined in `.simla.yaml` and executed using the `simla workflow` command. They support:

- Sequential task execution
- Parallel branches
- Conditional branching (Choice)
- Error handling (Retry/Catch)
- Data transformation (InputPath, OutputPath, ResultPath)
- Wait states

## Running Workflows

### List Available Workflows

```bash
simla workflow list
```

### Execute a Workflow

```bash
# With inline input
simla workflow run my-workflow --input '{"key":"value"}'

# With input from file
simla workflow run my-workflow --file event.json

# Pretty-print output
simla workflow run my-workflow --input '{}' --pretty
```

## State Types

### Task

Invokes a Lambda service and waits for completion.

```yaml
TaskState:
  Type: Task
  Resource: my-service      # Service name (required)
  Next: next-state
  # or
  End: true
  
  # Optional
  TimeoutSeconds: 60
  InputPath: "$.input"
  OutputPath: "$.output"
  ResultPath: "$.result"
  Retry:
    - ErrorEquals: ["States.Timeout"]
      IntervalSeconds: 2
      MaxAttempts: 3
      BackoffRate: 2.0
      Jitter: true
  Catch:
    - ErrorEquals: ["States.ALL"]
      Next: error-handler
      ResultPath: "$.error"
```

### Pass

Passes input to output without invoking a service. Useful for data transformation.

```yaml
TransformData:
  Type: Pass
  Result:
    status: "processed"
    timestamp: "2025-01-15T10:00:00Z"
  ResultPath: "$.metadata"
  Next: next-state

# Or transform existing data
CopyField:
  Type: Pass
  InputPath: "$.user"
  OutputPath: "$.customer"
  Next: next-state
```

### Choice

Branches based on conditions. Like a switch statement.

```yaml
RouteOrder:
  Type: Choice
  Choices:
    - Variable: "$.order.total"
      NumericGreaterThan: 1000
      Next: premium-support
    - Variable: "$.order.type"
      StringEquals: "express"
      Next: express-processing
    - Variable: "$.order.priority"
      BooleanEquals: true
      Next: priority-queue
  Default: standard-processing
```

#### Comparison Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `StringEquals` | Exact string match | `StringEquals: "premium"` |
| `StringLessThan` | Lexicographic less than | `StringLessThan: "z"` |
| `StringGreaterThan` | Lexicographic greater than | `StringGreaterThan: "a"` |
| `StringMatches` | Glob pattern with `*` | `StringMatches: "user-*" |
| `NumericEquals` | Exact numeric match | `NumericEquals: 100` |
| `NumericLessThan` | Numeric less than | `NumericLessThan: 1000` |
| `NumericGreaterThan` | Numeric greater than | `NumericGreaterThan: 0` |
| `BooleanEquals` | Boolean match | `BooleanEquals: true` |
| `IsNull` | Field is null | `IsNull: true` |
| `IsPresent` | Field exists | `IsPresent: true` |
| `IsString` | Field is string | `IsString: true` |
| `IsNumeric` | Field is number | `IsNumeric: true` |
| `IsBoolean` | Field is boolean | `IsBoolean: true` |

#### Complex Conditions

```yaml
ComplexCondition:
  Type: Choice
  Choices:
    # AND: all conditions must be true
    - And:
        - Variable: "$.user.age"
          NumericGreaterThan: 18
        - Variable: "$.user.country"
          StringEquals: "US"
      Next: eligible
    
    # OR: any condition must be true
    - Or:
        - Variable: "$.user.type"
          StringEquals: "premium"
        - Variable: "$.user.type"
          StringEquals: "vip"
      Next: vip-processing
    
    # NOT: negate condition
    - Not:
        Variable: "$.user.blocked"
        BooleanEquals: true
      Next: allowed
    
    # Nested
    - And:
        - Variable: "$.order.total"
          NumericGreaterThan: 0
        - Or:
            - Variable: "$.order.type"
              StringEquals: "digital"
            - Variable: "$.order.shippable"
              BooleanEquals: true
      Next: processable
```

### Parallel

Executes multiple branches concurrently.

```yaml
ProcessOrder:
  Type: Parallel
  Branches:
    - StartAt: charge-customer
      States:
        charge-customer:
          Type: Task
          Resource: payment-service
          End: true
    
    - StartAt: notify-warehouse
      States:
        notify-warehouse:
          Type: Task
          Resource: warehouse-service
          End: true
    
    - StartAt: send-confirmation
      States:
        send-confirmation:
          Type: Task
          Resource: notification-service
          End: true
  
  ResultPath: "$.parallel-results"
  Next: finalize
```

### Wait

Pauses execution for a specified duration or until a timestamp.

```yaml
# Wait for a fixed duration
WaitFiveSeconds:
  Type: Wait
  Seconds: 5
  Next: continue

# Wait for a duration from input
WaitFromInput:
  Type: Wait
  SecondsPath: "$.delaySeconds"
  Next: continue

# Wait until a specific timestamp
WaitUntilNoon:
  Type: Wait
  Timestamp: "2025-01-15T12:00:00Z"
  Next: continue

# Wait until timestamp from input
WaitUntil:
  Type: Wait
  TimestampPath: "$.scheduledTime"
  Next: continue
```

### Succeed

Ends the workflow successfully.

```yaml
Success:
  Type: Succeed
  # Optional output
  Output: "Task completed successfully"
```

### Fail

Ends the workflow with an error.

```yaml
ValidationFailed:
  Type: Fail
  Error: ValidationError
  Cause: "Order data is invalid"
```

Dynamic cause from input:
```yaml
ValidationFailed:
  Type: Fail
  Error: ValidationError
  CausePath: "$.error.message"
```

## Data Flow

Workflows pass JSON data between states using three path mechanisms.

### InputPath

Selects a portion of the input to pass to the state.

```yaml
# Input: {"order": {"id": "123"}, "metadata": {"timestamp": "..."}}
GetOrderId:
  Type: Pass
  InputPath: "$.order"
  OutputPath: "$"
  Result: {"status": "received"}
# Output: {"status": "received"}
```

### OutputPath

Filters the state output before passing to the next state.

```yaml
# Input to state: {"result": {...}, "debug": {...}}
FilterOutput:
  Type: Pass
  InputPath: "$"
  OutputPath: "$.result"
# Output: {...} (debug info removed)
```

### ResultPath

Merges the state's result into the input.

```yaml
# Input: {"orderId": "123"}
# Task result: {"transactionId": "tx-456", "amount": 99.99}
MergeResult:
  Type: Task
  Resource: payment-service
  ResultPath: "$.payment"
# Output: {"orderId": "123", "payment": {"transactionId": "tx-456", "amount": 99.99}}
```

## Error Handling

### Retry

Retry failed tasks with exponential backoff.

```yaml
RobustTask:
  Type: Task
  Resource: unreliable-service
  Retry:
    # Retry on timeout
    - ErrorEquals: ["States.Timeout"]
      IntervalSeconds: 2
      MaxAttempts: 3
      BackoffRate: 2.0
      Jitter: true
    
    # Retry on specific errors
    - ErrorEquals: ["ServiceUnavailable"]
      IntervalSeconds: 5
      MaxAttempts: 5
      BackoffRate: 1.5
    
    # Retry on any error
    - ErrorEquals: ["States.TaskFailed"]
      IntervalSeconds: 1
      MaxAttempts: 2
  Next: continue
```

**Error Types:**

| Error Type | Description |
|------------|-------------|
| `States.ALL` | Match any error |
| `States.Timeout` | Task exceeded TimeoutSeconds |
| `States.TaskFailed` | Task threw an exception |
| `States.heartbeat` | Heartbeat timeout |
| Custom | Match Lambda response errors |

### Catch

Handle errors with fallback states.

```yaml
WithCatch:
  Type: Task
  Resource: primary-service
  Retry:
    - ErrorEquals: ["States.ALL"]
      MaxAttempts: 1
  Catch:
    - ErrorEquals: ["States.TaskFailed"]
      Next: fallback-service
      ResultPath: "$.error"
    - ErrorEquals: ["States.Timeout"]
      Next: timeout-handler
      ResultPath: "$.error"
    - ErrorEquals: ["States.ALL"]
      Next: generic-error-handler
```

## Complete Examples

### Linear Workflow

```yaml
workflows:
  process-order:
    StartAt: validate
    States:
      validate:
        Type: Task
        Resource: validation-service
        InputPath: "$.order"
        ResultPath: "$.validation"
        Next: process
      
      process:
        Type: Task
        Resource: processing-service
        InputPath: "$.order"
        ResultPath: "$.processed"
        Next: notify
      
      notify:
        Type: Task
        Resource: notification-service
        InputPath: "$.order.id"
        End: true
```

### Order Processing Pipeline

```yaml
workflows:
  order-pipeline:
    Comment: "Complete order processing with parallel steps"
    StartAt: validate-order
    States:
      validate-order:
        Type: Task
        Resource: validation-service
        InputPath: "$.order"
        ResultPath: "$.validation"
        Next: check-inventory
      
      check-inventory:
        Type: Choice
        Choices:
          - Variable: "$.validation.available"
            BooleanEquals: true
            Next: process-order
        Default: handle-out-of-stock
      
      handle-out-of-stock:
        Type: Pass
        Result:
          status: "out_of_stock"
          message: "Items not available"
        End: true
      
      process-order:
        Type: Parallel
        Branches:
          - StartAt: process-payment
            States:
              process-payment:
                Type: Task
                Resource: payment-service
                ResultPath: "$.payment"
                End: true
          
          - StartAt: reserve-inventory
            States:
              reserve-inventory:
                Type: Task
                Resource: inventory-service
                ResultPath: "$.inventory"
                End: true
          
          - StartAt: generate-packing-list
            States:
              generate-packing-list:
                Type: Task
                Resource: warehouse-service
                ResultPath: "$.packing"
                End: true
        ResultPath: "$.parallel-results"
        Next: fulfill-order
      
      fulfill-order:
        Type: Task
        Resource: fulfillment-service
        ResultPath: "$.fulfillment"
        Next: send-confirmation
      
      send-confirmation:
        Type: Task
        Resource: notification-service
        InputPath: "$.order"
        End: true
```

### Scheduled Workflow

```yaml
workflows:
  daily-report:
    Comment: "Generate and send daily report"
    StartAt: fetch-data
    States:
      fetch-data:
        Type: Task
        Resource: data-service
        ResultPath: "$.report-data"
        Next: generate-report
      
      generate-report:
        Type: Task
        Resource: report-service
        InputPath: "$.report-data"
        ResultPath: "$.report"
        Next: deliver-report
      
      deliver-report:
        Type: Choice
        Choices:
          - Variable: "$.report.format"
            StringEquals: "email"
            Next: send-email
          - Variable: "$.report.format"
            StringEquals: "slack"
            Next: send-slack
        Default: store-report
      
      send-email:
        Type: Task
        Resource: email-service
        End: true
      
      send-slack:
        Type: Task
        Resource: slack-service
        End: true
      
      store-report:
        Type: Task
        Resource: storage-service
        End: true
```

## Best Practices

### 1. Always Define Timeouts

```yaml
LongRunningTask:
  Type: Task
  Resource: service
  TimeoutSeconds: 300  # 5 minutes
```

### 2. Implement Retry Logic

```yaml
Task:
  Type: Task
  Resource: service
  Retry:
    - ErrorEquals: ["States.ALL"]
      MaxAttempts: 3
      IntervalSeconds: 2
      BackoffRate: 2.0
      Jitter: true
```

### 3. Handle Errors Explicitly

```yaml
Task:
  Type: Task
  Resource: service
  Catch:
    - ErrorEquals: ["States.ALL"]
      Next: error-handler
```

### 4. Use ResultPath to Preserve Context

```yaml
Task:
  Type: Task
  Resource: service
  ResultPath: "$.task-result"  # Preserve original input
```

### 5. Limit Parallel Branch Count

AWS recommends limiting parallel branches to 50 or fewer.

## Troubleshooting

### Workflow Not Found

Ensure the workflow is defined in `.simla.yaml` under `workflows:`.

### Service Not Invoked

Check that the `Resource` value matches a defined service.

### Infinite Loop

Ensure choice states have a `Default` or all conditions eventually lead to terminal states.

### Data Not Passed Correctly

Verify `InputPath`, `OutputPath`, and `ResultPath` are correct JSON pointers.

## Next Steps

- [Configuration Reference](configuration.md) - All config options
- [Triggers](triggers.md) - Event-driven workflows
- [Architecture](architecture.md) - How workflows execute
