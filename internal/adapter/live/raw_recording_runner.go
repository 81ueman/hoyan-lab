package live

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type RawRecordingRunner struct {
	Runner Runner
	Dir    string
	seq    int
}

func NewRawRecordingRunner(runner Runner, dir string) *RawRecordingRunner {
	return &RawRecordingRunner{Runner: runner, Dir: dir}
}

func (r *RawRecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	data, err := r.Runner.Run(ctx, name, args...)
	if err == nil {
		r.seq++
		_ = os.MkdirAll(r.Dir, 0o755)
		_ = os.WriteFile(filepath.Join(r.Dir, fmt.Sprintf("%03d.%s.raw", r.seq, sanitizeRawName(append([]string{name}, args...)))), data, 0o644)
	}
	return data, err
}

func sanitizeRawName(parts []string) string {
	joined := strings.Join(parts, ".")
	var b strings.Builder
	lastDot := false
	for _, r := range joined {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDot = false
			continue
		}
		if !lastDot {
			b.WriteByte('.')
			lastDot = true
		}
	}
	return strings.Trim(b.String(), ".")
}
