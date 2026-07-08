package srlinuxjson

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// testSession provides a fake Session for testing ExecJSON.
type testSession struct {
	execFn    func(ctx context.Context, args ...string) ([]byte, error)
	execTTYFn func(ctx context.Context, args ...string) ([]byte, error)
}

func (s testSession) Exec(ctx context.Context, args ...string) ([]byte, error) {
	return s.execFn(ctx, args...)
}

func (s testSession) ExecTTY(ctx context.Context, args ...string) ([]byte, error) {
	if s.execTTYFn == nil {
		return nil, errors.New("ExecTTY not implemented")
	}
	return s.execTTYFn(ctx, args...)
}

func TestExecJSONRetriesMalformedOutput(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	defer func() { retryDelay = oldDelay }()

	calls := 0
	session := testSession{
		execFn: func(ctx context.Context, args ...string) ([]byte, error) {
			calls++
			if len(args) < 4 || args[0] != "sr_cli" || args[1] != "--output-format" {
				t.Fatalf("unexpected args: %v", args)
			}
			if calls == 1 {
				return []byte(`{"header":`), nil
			}
			return []byte(`{"header":[]}`), nil
		},
	}
	data, err := ExecJSON(context.Background(), session, "show", "version")
	if err != nil {
		t.Fatalf("ExecJSON() error = %v", err)
	}
	if string(data) != `{"header":[]}` {
		t.Fatalf("data = %q", data)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want retry after malformed output", calls)
	}
}

func TestExecJSONFallsBackToTTY(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	defer func() { retryDelay = oldDelay }()

	var ttyCalled bool
	session := testSession{
		execFn: func(ctx context.Context, args ...string) ([]byte, error) {
			return nil, nil
		},
		execTTYFn: func(ctx context.Context, args ...string) ([]byte, error) {
			ttyCalled = true
			if len(args) < 4 || args[0] != "sr_cli" || args[1] != "--output-format" {
				t.Fatalf("unexpected TTY args: %v", args)
			}
			return []byte(`{"header":[]}`), nil
		},
	}
	data, err := ExecJSON(context.Background(), session, "show", "version")
	if err != nil {
		t.Fatalf("ExecJSON() error = %v", err)
	}
	if string(data) != `{"header":[]}` {
		t.Fatalf("data = %q", data)
	}
	if !ttyCalled {
		t.Fatalf("TTY fallback was not called")
	}
}

func TestExecJSONReportsMalformedOutputPreview(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	defer func() { retryDelay = oldDelay }()

	session := testSession{
		execFn: func(ctx context.Context, args ...string) ([]byte, error) {
			return []byte(`{"header":`), nil
		},
	}
	_, err := ExecJSON(context.Background(), session, "show", "version")
	if err == nil {
		t.Fatalf("ExecJSON() succeeded unexpectedly")
	}
	for _, want := range []string{"malformed JSON", "bytes=10", `preview="{\"header\":"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}
