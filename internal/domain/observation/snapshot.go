package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

type NetworkSnapshot struct {
	Metadata SnapshotMetadata `json:"metadata"`
	Nodes    []NodeSnapshot   `json:"nodes"`
}

type SnapshotMetadata struct {
	ID          string            `json:"id,omitempty"`
	Source      string            `json:"source,omitempty"`
	CollectedAt string            `json:"collected_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type NodeSnapshot struct {
	Node model.NodeID  `json:"node"`
	VRFs []VRFSnapshot `json:"vrfs"`
}

type VRFSnapshot struct {
	VRF VRFName `json:"vrf"`
	RIB RIB     `json:"rib"`
	FIB FIB     `json:"fib"`
}
