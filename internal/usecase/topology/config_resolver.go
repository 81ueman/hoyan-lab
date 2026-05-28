package topology

import (
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/labfile"
)

func resolveConfigPath(n labfile.Node) string {
	if n.StartupConfig != "" {
		return n.StartupConfig
	}
	for _, bind := range n.Binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && parts[1] == "/etc/frr/frr.conf" {
			return parts[0]
		}
		if len(parts) >= 2 && parts[1] == "/etc/frr" {
			return filepath.Join(parts[0], "frr.conf")
		}
	}
	return ""
}

func resolveNftablesConfigPath(n labfile.Node) string {
	for _, bind := range n.Binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && parts[1] == "/etc/hoyan/nftables.conf" {
			return parts[0]
		}
	}
	return ""
}
