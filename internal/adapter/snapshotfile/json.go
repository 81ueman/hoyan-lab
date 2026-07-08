package snapshotfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
)

type Repository struct{}

func NewRepository() Repository {
	return Repository{}
}

func (Repository) Load(path string) (*snapshotdomain.Snapshot, error) {
	return Load(path)
}

func Load(path string) (*snapshotdomain.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap snapshotdomain.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Version == "" {
		return nil, fmt.Errorf("snapshot %s has no version", path)
	}
	snap.Network = observation.NormalizeNetworkSnapshot(snap.Network)
	return &snap, nil
}

func Save(path string, snap *snapshotdomain.Snapshot) error {
	if path == "" || path == "-" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Marshal(snap *snapshotdomain.Snapshot) ([]byte, error) {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func LoadObservation(path string) (observation.NetworkSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return observation.NetworkSnapshot{}, err
	}
	var snap observation.NetworkSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return observation.NetworkSnapshot{}, err
	}
	return observation.NormalizeNetworkSnapshot(snap), nil
}

func SaveObservation(path string, snap observation.NetworkSnapshot) error {
	if path == "" || path == "-" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(observation.NormalizeNetworkSnapshot(snap))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := MarshalObservation(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func MarshalObservation(snap observation.NetworkSnapshot) ([]byte, error) {
	data, err := json.MarshalIndent(observation.NormalizeNetworkSnapshot(snap), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
