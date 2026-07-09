package intentdsl

import (
	"fmt"
	"os"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
)

// Load reads a .hoyan DSL file and returns a parsed intent.Document.
func Load(path string) (*intent.Document, error) {
	if !strings.HasSuffix(path, ".hoyan") {
		return nil, fmt.Errorf("%s: intent file must use .hoyan extension", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lex := newLexer(string(data), path)
	p := newParser(lex)
	return p.ParseDocument()
}
