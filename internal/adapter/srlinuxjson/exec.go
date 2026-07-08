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

// Session represents a session to a network device for running commands.
// This is the interface consumed by ExecJSON; it is intentionally minimal so
// that callers can pass any device session (docker exec, SSH, etc.).
type Session interface {
	Exec(ctx context.Context, args ...string) ([]byte, error)
}

// ttySession is an optional interface that sessions may implement to provide
// TTY-aware command execution (used as fallback when non-TTY exec returns
// empty or malformed output, as sometimes happens with SR Linux sr_cli).
type ttySession interface {
	Session
	ExecTTY(ctx context.Context, args ...string) ([]byte, error)
}

func ExecJSON(ctx context.Context, session Session, showArgs ...string) ([]byte, error) {
	commandText := strings.Join(showArgs, " ")
	args := append([]string{"sr_cli", "--output-format", "json", "--pagination", "off", "--"}, showArgs...)
	var last []byte
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		data, err := session.Exec(ctx, args...)
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

	if ts, ok := session.(ttySession); ok {
		data, ttyErr := ts.ExecTTY(ctx, args...)
		if ttyErr != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("sr_cli %s: %w", commandText, ttyErr)
			}
			return nil, malformedError(commandText, maxAttempts, last)
		}
		if validJSON(data) {
			return data, nil
		}
		return nil, malformedError(commandText, 1, data)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("sr_cli %s: %w", commandText, lastErr)
	}
	return nil, malformedError(commandText, maxAttempts, last)
}

func validJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && json.Valid(trimmed)
}

func malformedError(commandText string, attempts int, data []byte) error {
	return fmt.Errorf("sr_cli %s returned malformed JSON after %d attempts: bytes=%d preview=%q", commandText, attempts, len(data), previewBytes(data, 160))
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
