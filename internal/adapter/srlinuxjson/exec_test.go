package srlinuxjson

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

func TestExecJSONRetriesMalformedOutput(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	defer func() { retryDelay = oldDelay }()

	calls := 0
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		if name != "docker" || strings.Join(args[:4], " ") != "exec -i srl1 sr_cli" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		if calls == 1 {
			return []byte(`{"header":`), nil
		}
		return []byte(`{"header":[]}`), nil
	})
	data, err := ExecJSON(context.Background(), runner, "srl1", "show", "version")
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

	var ttyCommand string
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch {
		case cmd == "docker exec -i srl1 sr_cli --output-format json --pagination off -- show version":
			return nil, nil
		case strings.HasPrefix(cmd, "script -q /dev/null -c docker exec -it 'srl1' sr_cli --output-format json --pagination off -- 'show' 'version'"):
			ttyCommand = cmd
			return []byte(`{"header":[]}`), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	})
	data, err := ExecJSON(context.Background(), runner, "srl1", "show", "version")
	if err != nil {
		t.Fatalf("ExecJSON() error = %v", err)
	}
	if string(data) != `{"header":[]}` {
		t.Fatalf("data = %q", data)
	}
	if ttyCommand == "" {
		t.Fatalf("TTY fallback was not called")
	}
}

func TestExecJSONReportsMalformedOutputPreview(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	defer func() { retryDelay = oldDelay }()

	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "docker":
			return []byte(`{"header":`), nil
		case "script":
			return []byte(`{"tty":`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	})
	_, err := ExecJSON(context.Background(), runner, "srl1", "show", "version")
	if err == nil {
		t.Fatalf("ExecJSON() succeeded unexpectedly")
	}
	for _, want := range []string{"malformed JSON", "bytes=7", `preview="{\"tty\":"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}
