package queryfile

import (
	"fmt"
	"os"

	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*query.Queries, error) {
	var queries query.Queries
	if err := loadYAML(path, &queries); err != nil {
		return nil, err
	}
	return &queries, nil
}

func loadYAML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
