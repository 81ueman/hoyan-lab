package model

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadQueries(path string) (*Queries, error) {
	var queries Queries
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
