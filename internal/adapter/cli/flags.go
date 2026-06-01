package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func addLabFlag(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVar(value, "lab", "", "scenario lab directory or name under labs/")
}

func addTopologyFlag(cmd *cobra.Command, value *string, usage string) {
	cmd.Flags().StringVar(value, "topology", defaultTopologyPath, usage)
}

func resolveLabInputs(cmd *cobra.Command, labPath string, topologyPath *string) error {
	if labPath == "" {
		return nil
	}
	labDir, err := resolveLabDir(labPath)
	if err != nil {
		return err
	}
	if topologyPath != nil && !cmd.Flags().Changed("topology") {
		*topologyPath = filepath.Join(labDir, labTopologyFile)
	}
	return nil
}

func resolveLabDir(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("--lab is empty")
	}
	candidates := []string{raw}
	if !strings.ContainsRune(raw, filepath.Separator) && !filepath.IsAbs(raw) {
		candidates = append(candidates, filepath.Join(defaultLabsDir, raw))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return filepath.Clean(candidate), nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("lab %q not found", raw)
}
