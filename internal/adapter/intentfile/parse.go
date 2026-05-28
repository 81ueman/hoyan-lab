package intentfile

import (
	"fmt"
	"os"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*intent.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc intent.Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &doc, nil
}
