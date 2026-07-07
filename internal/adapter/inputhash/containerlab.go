package inputhash

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"

	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	"gopkg.in/yaml.v3"
)

type Provider struct{}

func NewProvider() Provider {
	return Provider{}
}

func (Provider) InputHashes(topologyPath string) (snapshotdomain.InputHashSet, error) {
	return InputHashes(topologyPath)
}

func InputHashes(topologyPath string) (snapshotdomain.InputHashSet, error) {
	topoHash, err := fileSHA256(topologyPath)
	if err != nil {
		return snapshotdomain.InputHashSet{}, err
	}
	configs, err := configPaths(topologyPath)
	if err != nil {
		return snapshotdomain.InputHashSet{}, err
	}
	hashes := map[string]string{}
	root := filepath.Dir(topologyPath)
	for _, path := range configs {
		full := path
		if !filepath.IsAbs(full) {
			full = filepath.Join(root, path)
		}
		sum, err := fileSHA256(full)
		if err != nil {
			return snapshotdomain.InputHashSet{}, err
		}
		hashes[filepath.ToSlash(path)] = sum
	}
	return snapshotdomain.InputHashSet{TopologyHash: topoHash, ConfigHashes: hashes}, nil
}

func (Provider) CheckHashes(topologyPath string, snap *snapshotdomain.Snapshot) (snapshotdomain.HashCheckResult, error) {
	return CheckHashes(topologyPath, snap)
}

func CheckHashes(topologyPath string, snap *snapshotdomain.Snapshot) (snapshotdomain.HashCheckResult, error) {
	hashes, err := InputHashes(topologyPath)
	if err != nil {
		return snapshotdomain.HashCheckResult{}, err
	}
	var result snapshotdomain.HashCheckResult
	if snap.TopologyHash != "" && hashes.TopologyHash != snap.TopologyHash {
		result.Mismatches = append(result.Mismatches, snapshotdomain.HashMismatch{Path: topologyPath, Want: snap.TopologyHash, Got: hashes.TopologyHash})
	}
	for path, want := range snap.ConfigHashes {
		got, ok := hashes.ConfigHashes[path]
		if !ok {
			result.Missing = append(result.Missing, path)
			continue
		}
		if got != want {
			result.Mismatches = append(result.Mismatches, snapshotdomain.HashMismatch{Path: path, Want: want, Got: got})
		}
	}
	sort.Slice(result.Mismatches, func(i, j int) bool { return result.Mismatches[i].Path < result.Mismatches[j].Path })
	sort.Strings(result.Missing)
	return result, nil
}

type clabHashFile struct {
	Topology struct {
		Nodes map[string]struct {
			Binds         []string `yaml:"binds"`
			StartupConfig string   `yaml:"startup-config"`
		} `yaml:"nodes"`
	} `yaml:"topology"`
}

func configPaths(topologyPath string) ([]string, error) {
	data, err := os.ReadFile(topologyPath)
	if err != nil {
		return nil, err
	}
	var raw clabHashFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, node := range raw.Topology.Nodes {
		if node.StartupConfig != "" {
			seen[filepath.Clean(node.StartupConfig)] = true
		}
		for _, bind := range node.Binds {
			parts := strings.Split(bind, ":")
			if len(parts) < 2 {
				continue
			}
			target := parts[1]
			if target == "/etc/frr/frr.conf" || target == "/etc/frr/daemons" || target == "/etc/frr/vtysh.conf" || target == "/etc/hoyan/nftables.conf" {
				seen[filepath.Clean(parts[0])] = true
			}
			if target == "/etc/frr" {
				seen[filepath.Clean(filepath.Join(parts[0], "frr.conf"))] = true
				seen[filepath.Clean(filepath.Join(parts[0], "daemons"))] = true
				seen[filepath.Clean(filepath.Join(parts[0], "vtysh.conf"))] = true
			}
		}
	}
	var out []string
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
