package traffic

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// DSCP represents a Differentiated Services Code Point value (0-63).
type DSCP uint8

const (
	// DSCPDefault is the standard DSCP value (0).
	DSCPDefault DSCP = 0
	// DSCPAF11 is Assured Forwarding 11 (DSCP 10).
	DSCPAF11 DSCP = 10
	// DSCPAF12 is Assured Forwarding 12 (DSCP 12).
	DSCPAF12 DSCP = 12
	// DSCPAF13 is Assured Forwarding 13 (DSCP 14).
	DSCPAF13 DSCP = 14
	// DSCPAF21 is Assured Forwarding 21 (DSCP 18).
	DSCPAF21 DSCP = 18
	// DSCPAF22 is Assured Forwarding 22 (DSCP 20).
	DSCPAF22 DSCP = 20
	// DSCPAF23 is Assured Forwarding 23 (DSCP 22).
	DSCPAF23 DSCP = 22
	// DSCPAF31 is Assured Forwarding 31 (DSCP 26).
	DSCPAF31 DSCP = 26
	// DSCPAF32 is Assured Forwarding 32 (DSCP 28).
	DSCPAF32 DSCP = 28
	// DSCPAF33 is Assured Forwarding 33 (DSCP 30).
	DSCPAF33 DSCP = 30
	// DSCPAF41 is Assured Forwarding 41 (DSCP 34).
	DSCPAF41 DSCP = 34
	// DSCPAF42 is Assured Forwarding 42 (DSCP 36).
	DSCPAF42 DSCP = 36
	// DSCPAF43 is Assured Forwarding 43 (DSCP 38).
	DSCPAF43 DSCP = 38
	// DSCPEF is Expedited Forwarding (DSCP 46).
	DSCPEF DSCP = 46
	// DSCPCS0 through DSCPCS7 are Class Selector values.
	DSCPCS0 DSCP = 0
	DSCPCS1 DSCP = 8
	DSCPCS2 DSCP = 16
	DSCPCS3 DSCP = 24
	DSCPCS4 DSCP = 32
	DSCPCS5 DSCP = 40
	DSCPCS6 DSCP = 48
	DSCPCS7 DSCP = 56
)

// SampledFlow represents a flow with its DSCP value for classification.
type SampledFlow struct {
	Flow
	DSCP DSCP
}

// FlowEquivalenceClassKey is the key for grouping flows into equivalence classes.
type FlowEquivalenceClassKey struct {
	PrefixClassID model.PrefixClassID
	Protocol      string
	SrcPort       model.PortSet
	DstPort       model.PortSet
	DSCP          DSCP
	IngressIface  string
	EgressIface   string
}

// FlowEquivalenceClass groups flows with the same forwarding behavior.
type FlowEquivalenceClass struct {
	Key   FlowEquivalenceClassKey
	Flows []SampledFlow
	// TotalBytes is the total traffic volume for this class.
	TotalBytes uint64
}

// SamplingConfig configures flow sampling behavior.
type SamplingConfig struct {
	// Rate is the fraction of flows to sample (0.0-1.0). 1.0 = all flows.
	Rate float64
	// Strategy selects the sampling strategy.
	Strategy SamplingStrategy
}

// SamplingStrategy defines how flows are sampled.
type SamplingStrategy string

const (
	// SamplingRandom randomly samples flows with the configured rate.
	SamplingRandom SamplingStrategy = "random"
	// SamplingTopNByBytes samples the top N flows by bytes.
	SamplingTopNByBytes SamplingStrategy = "top_n_by_bytes"
)

// DefaultSamplingConfig returns a default sampling configuration (all flows).
func DefaultSamplingConfig() SamplingConfig {
	return SamplingConfig{
		Rate:     1.0,
		Strategy: SamplingRandom,
	}
}

// Classifier classifies flows into equivalence classes.
type Classifier struct {
	config SamplingConfig
}

// NewClassifier creates a new classifier with the given sampling config.
func NewClassifier(config SamplingConfig) *Classifier {
	return &Classifier{config: config}
}

// ClassifyFlows groups flows into equivalence classes based on their forwarding
// properties. Returns a slice of FlowEquivalenceClass sorted by total bytes.
func (c *Classifier) ClassifyFlows(flows []SampledFlow) []FlowEquivalenceClass {
	sampled := c.sampleFlows(flows)
	classes := c.groupByKey(sampled)
	return classes
}

// ClassifyFlowsFromPacketClass creates equivalence classes from flows matching
// a given packet class. This bridges the existing PacketClass concept with
// the DSCP-aware classification.
func (c *Classifier) ClassifyFlowsFromPacketClass(pc model.PacketClass, flows []SampledFlow) []FlowEquivalenceClass {
	var matching []SampledFlow
	for _, f := range flows {
		if c.flowMatchesPacketClass(f, pc) {
			matching = append(matching, f)
		}
	}
	return c.ClassifyFlows(matching)
}

// sampleFlows applies the configured sampling strategy.
func (c *Classifier) sampleFlows(flows []SampledFlow) []SampledFlow {
	if c.config.Rate >= 1.0 {
		return flows
	}
	if c.config.Rate <= 0.0 {
		return nil
	}

	switch c.config.Strategy {
	case SamplingTopNByBytes:
		return c.sampleTopN(flows)
	default: // SamplingRandom
		return c.sampleRandom(flows)
	}
}

func (c *Classifier) sampleRandom(flows []SampledFlow) []SampledFlow {
	var sampled []SampledFlow
	for _, f := range flows {
		if rand.Float64() < c.config.Rate {
			sampled = append(sampled, f)
		}
	}
	return sampled
}

func (c *Classifier) sampleTopN(flows []SampledFlow) []SampledFlow {
	if len(flows) == 0 {
		return nil
	}

	n := int(float64(len(flows)) * c.config.Rate)
	if n < 1 {
		n = 1
	}
	if n >= len(flows) {
		return flows
	}

	sorted := make([]SampledFlow, len(flows))
	copy(sorted, flows)
	sort.Slice(sorted, func(i, j int) bool {
		// Sort by DstIP bytes as a proxy for volume
		return sorted[i].DstIP.String() < sorted[j].DstIP.String()
	})
	return sorted[:n]
}

// flowEquivalenceKey computes the equivalence class key for a flow.
func (c *Classifier) flowEquivalenceKey(f SampledFlow, pc model.PacketClass) FlowEquivalenceClassKey {
	return FlowEquivalenceClassKey{
		PrefixClassID: pc.PrefixClassID,
		Protocol:      f.Protocol,
		SrcPort:       model.ExactPort(int(f.SrcPort)),
		DstPort:       model.ExactPort(int(f.DstPort)),
		DSCP:          f.DSCP,
		IngressIface:  pc.IngressInterface,
		EgressIface:   pc.EgressInterface,
	}
}

// groupByKey groups sampled flows by their equivalence class key.
func (c *Classifier) groupByKey(flows []SampledFlow) []FlowEquivalenceClass {
	groups := map[FlowEquivalenceClassKey]*FlowEquivalenceClass{}
	keysInOrder := []FlowEquivalenceClassKey{}

	// Use a dummy packet class for key computation
	dummyPC := model.PacketClass{}

	for _, f := range flows {
		key := c.flowEquivalenceKey(f, dummyPC)
		if _, ok := groups[key]; !ok {
			groups[key] = &FlowEquivalenceClass{
				Key:   key,
				Flows: []SampledFlow{},
			}
			keysInOrder = append(keysInOrder, key)
		}
		groups[key].Flows = append(groups[key].Flows, f)
		groups[key].TotalBytes += 1500 // Assume ~1500 bytes per flow
	}

	result := make([]FlowEquivalenceClass, 0, len(groups))
	for _, k := range keysInOrder {
		result = append(result, *groups[k])
	}

	// Sort by total bytes descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalBytes > result[j].TotalBytes
	})

	return result
}

// flowMatchesPacketClass checks if a flow's 5-tuple matches a packet class spec.
func (c *Classifier) flowMatchesPacketClass(f SampledFlow, pc model.PacketClass) bool {
	// Check protocol
	if pc.Protocol != "" && pc.Protocol != f.Protocol {
		return false
	}

	// Check destination prefix
	if pc.DstSet != nil && !pc.DstSet.ContainsAddr(f.DstIP) {
		return false
	}

	// Check source port
	if pc.SrcPort != nil && !pc.SrcPort.Contains(int(f.SrcPort)) {
		return false
	}

	// Check destination port
	if pc.DstPort != nil && !pc.DstPort.Contains(int(f.DstPort)) {
		return false
	}

	return true
}

// FlowEquivalenceClassKeyFromPacketClass derives a classification key from a PacketClass.
// This allows the existing PacketClass to be used with DSCP-aware classification.
func FlowEquivalenceClassKeyFromPacketClass(pc model.PacketClass, dscp DSCP) FlowEquivalenceClassKey {
	return FlowEquivalenceClassKey{
		PrefixClassID: pc.PrefixClassID,
		Protocol:      pc.Protocol,
		SrcPort:       pc.SrcPort,
		DstPort:       pc.DstPort,
		DSCP:          dscp,
		IngressIface:  pc.IngressInterface,
		EgressIface:   pc.EgressInterface,
	}
}

// String returns a human-readable representation of a classification key.
func (k FlowEquivalenceClassKey) String() string {
	return fmt.Sprintf("EC{prefix=%d proto=%s srcPort=%s dstPort=%s dscp=%d ifaces=%s/%s}",
		k.PrefixClassID, k.Protocol,
		portSetString(k.SrcPort), portSetString(k.DstPort),
		k.DSCP, k.IngressIface, k.EgressIface)
}

func portSetString(ps model.PortSet) string {
	if ps == nil {
		return "any"
	}
	return ps.String()
}
