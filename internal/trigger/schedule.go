package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/nyambati/simla/internal/config"
)

// scheduleTrigger fires a Lambda on a fixed interval derived from an AWS rate()
// or cron() expression.
type scheduleTrigger struct {
	base
	interval   time.Duration
	expression string
}

func newSchedule(trig config.Trigger, b base) (Source, error) {
	if trig.Expression == "" {
		return nil, fmt.Errorf("schedule trigger for service %s: expression is required", b.serviceName)
	}
	interval, err := parseExpression(trig.Expression)
	if err != nil {
		return nil, fmt.Errorf("schedule trigger for service %s: %w", b.serviceName, err)
	}
	return &scheduleTrigger{
		base:       b,
		interval:   interval,
		expression: trig.Expression,
	}, nil
}

func (s *scheduleTrigger) Start(ctx context.Context) error {
	s.logger.Infof("schedule trigger started for service %s (expression=%q, interval=%s)",
		s.serviceName, s.expression, s.interval)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Infof("schedule trigger stopped for service %s", s.serviceName)
			return nil
		case t := <-ticker.C:
			payload, err := buildSchedulePayload(s.expression, t)
			if err != nil {
				s.logger.WithError(err).Warn("failed to build schedule event payload")
				continue
			}
			s.invoke(ctx, payload)
		}
	}
}

// buildSchedulePayload constructs an events.CloudWatchEvent that mirrors what
// AWS EventBridge sends for a scheduled rule invocation.
func buildSchedulePayload(expression string, t time.Time) ([]byte, error) {
	evt := events.CloudWatchEvent{
		Version:    "0",
		ID:         uuid.NewString(),
		DetailType: "Scheduled Event",
		Source:     "aws.events",
		AccountID:  "012345678901",
		Time:       t,
		Region:     "us-east-1",
		Resources:  []string{fmt.Sprintf("arn:aws:events:us-east-1:012345678901:rule/simla-%s", expression)},
		Detail:     json.RawMessage("{}"),
	}
	return json.Marshal(evt)
}

// parseExpression converts an AWS rate() or cron() expression to a Go
// time.Duration. Cron expressions are approximated to their minimum firing
// period (the smallest non-zero field).
func parseExpression(expr string) (time.Duration, error) {
	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "rate(") && strings.HasSuffix(expr, ")") {
		return parseRate(expr[len("rate(") : len(expr)-1])
	}

	if strings.HasPrefix(expr, "cron(") && strings.HasSuffix(expr, ")") {
		return parseCron(expr[len("cron(") : len(expr)-1])
	}

	return 0, fmt.Errorf("unsupported expression %q: must start with rate() or cron()", expr)
}

// parseRate handles "N unit" where unit is minute/minutes/hour/hours/day/days.
func parseRate(inner string) (time.Duration, error) {
	parts := strings.Fields(inner)
	if len(parts) != 2 {
		return 0, fmt.Errorf("rate expression must be \"N unit\", got %q", inner)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("rate value must be a positive integer, got %q", parts[0])
	}
	unit := strings.ToLower(parts[1])
	switch unit {
	case "minute", "minutes":
		return time.Duration(n) * time.Minute, nil
	case "hour", "hours":
		return time.Duration(n) * time.Hour, nil
	case "day", "days":
		return time.Duration(n) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported rate unit %q; use minute(s), hour(s), or day(s)", parts[1])
	}
}

// parseCron approximates a cron expression by inspecting the smallest
// non-wildcard time field (minute → 1m, hour → 1h, day → 24h, etc.).
// This is intentionally simplified: full cron scheduling is out of scope for a
// local dev tool.
func parseCron(inner string) (time.Duration, error) {
	// AWS cron format: Minutes Hours Day-of-month Month Day-of-week Year
	fields := strings.Fields(inner)
	if len(fields) != 6 {
		return 0, fmt.Errorf("cron expression must have 6 fields (got %d): %q", len(fields), inner)
	}
	minutes, hours, dom := fields[0], fields[1], fields[2]
	switch {
	case minutes != "*" && minutes != "?":
		return time.Minute, nil
	case hours != "*" && hours != "?":
		return time.Hour, nil
	case dom != "*" && dom != "?":
		return 24 * time.Hour, nil
	default:
		return 24 * time.Hour, nil
	}
}
