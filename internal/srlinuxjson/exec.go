package srlinuxjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxAttempts = 3

var retryDelay = 250 * time.Millisecond

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

func ExecJSON(ctx context.Context, runner Runner, containerName string, showArgs ...string) ([]byte, error) {
	args := append([]string{"exec", "-i", containerName, "sr_cli", "--output-format", "json", "--pagination", "off", "--"}, showArgs...)
	commandText := strings.Join(showArgs, " ")
	var last []byte
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		data, err := runner.Run(ctx, "docker", args...)
		if err == nil && validJSON(data) {
			return data, nil
		}
		last = data
		lastErr = err
		if attempt < maxAttempts {
			if err := sleepContext(ctx, retryDelay); err != nil {
				return nil, err
			}
		}
	}

	data, ttyErr := runTTY(ctx, runner, containerName, showArgs...)
	if ttyErr != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("docker exec -i/it %s sr_cli %s: %w", containerName, commandText, ttyErr)
		}
		return nil, malformedError("docker exec -i/it", containerName, commandText, maxAttempts, last)
	}
	if validJSON(data) {
		return data, nil
	}
	return nil, malformedError("docker exec -it", containerName, commandText, 1, data)
}

func runTTY(ctx context.Context, runner Runner, containerName string, showArgs ...string) ([]byte, error) {
	command := "docker exec -it " + shellQuote(containerName) + " sr_cli --output-format json --pagination off -- " + shellJoin(showArgs)
	return runner.Run(ctx, "script", "-q", "/dev/null", "-c", command)
}

func validJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && json.Valid(trimmed)
}

func malformedError(invocation, containerName, commandText string, attempts int, data []byte) error {
	return fmt.Errorf("%s %s sr_cli %s returned malformed JSON after %d attempts: bytes=%d preview=%q", invocation, containerName, commandText, attempts, len(data), previewBytes(data, 160))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func previewBytes(data []byte, limit int) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) <= limit {
		return string(trimmed)
	}
	return string(trimmed[:limit]) + "..."
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
