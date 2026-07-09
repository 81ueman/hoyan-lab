package intentdsl

import (
	"os"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
)

// Load reads a .hoyan DSL file and returns a parsed intent.Document.
func Load(path string) (*intent.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lex := newLexer(string(data), path)
	p := newParser(lex)
	return p.ParseDocument()
}
