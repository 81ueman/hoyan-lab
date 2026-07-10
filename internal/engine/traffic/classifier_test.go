package traffic

import (
	"net/netip"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestClassifierDSCPKey(t *testing.T) {
	classifier := NewClassifier(DefaultSamplingConfig())

	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 8080}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.3"), DstIP: netip.MustParseAddr("10.0.0.4"), Protocol: "tcp", SrcPort: 80, DstPort: 8080}, DSCP: DSCPEF},
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) != 2 {
		t.Errorf("expected 2 classes (different DSCP), got %d", len(classes))
	}

	// Verify DSCP is part of the classification key
	dscpValues := map[DSCP]bool{}
	for _, c := range classes {
		dscpValues[c.Key.DSCP] = true
	}
	if !dscpValues[DSCPDefault] {
		t.Errorf("expected DSCP default class")
	}
	if !dscpValues[DSCPEF] {
		t.Errorf("expected DSCP EF class")
	}
}

func TestClassifierGroupsByDSCP(t *testing.T) {
	classifier := NewClassifier(DefaultSamplingConfig())

	// Same 5-tuple but different DSCP values
	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.3"), DstIP: netip.MustParseAddr("10.0.0.4"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPAF11},
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) != 2 {
		t.Errorf("expected 2 classes, got %d", len(classes))
	}

	for _, c := range classes {
		switch c.Key.DSCP {
		case DSCPDefault:
			if len(c.Flows) != 2 {
				t.Errorf("expected 2 flows in DSCP default class, got %d", len(c.Flows))
			}
		case DSCPAF11:
			if len(c.Flows) != 1 {
				t.Errorf("expected 1 flow in DSCP AF11 class, got %d", len(c.Flows))
			}
		default:
			t.Errorf("unexpected DSCP value: %d", c.Key.DSCP)
		}
	}
}

func TestClassifierFlowMatchingPacketClass(t *testing.T) {
	classifier := NewClassifier(DefaultSamplingConfig())

	pc := model.PacketClass{
		PrefixClassID: 1,
		DstSet:        model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
		Protocol:      "tcp",
	}

	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.100"), Protocol: "tcp", SrcPort: 12345, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.2"), DstIP: netip.MustParseAddr("10.0.0.200"), Protocol: "udp", SrcPort: 53, DstPort: 443}, DSCP: DSCPEF},
		{Flow: Flow{SrcIP: netip.MustParseAddr("192.168.1.1"), DstIP: netip.MustParseAddr("192.168.1.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
	}

	classes := classifier.ClassifyFlowsFromPacketClass(pc, flows)
	// Only flows matching the packet class should be included (flows[0] matches tcp+10.0.0.0/24)
	if len(classes) != 1 {
		t.Errorf("expected 1 class (only tcp+10.0.0.0/24 matches), got %d", len(classes))
	}
}

func TestClassifierSamplingRandom(t *testing.T) {
	config := SamplingConfig{
		Rate:     0.5,
		Strategy: SamplingRandom,
	}
	classifier := NewClassifier(config)

	flows := make([]SampledFlow, 100)
	for i := range flows {
		flows[i] = SampledFlow{
			Flow: Flow{
				SrcIP:    netip.MustParseAddr("10.0.0.1"),
				DstIP:    netip.MustParseAddr("10.0.0.2"),
				Protocol: "tcp",
				SrcPort:  uint16(10000 + i),
				DstPort:  80,
			},
			DSCP: DSCPDefault,
		}
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) == 0 {
		t.Errorf("expected some flows to be sampled")
	}
	totalFlows := 0
	for _, c := range classes {
		totalFlows += len(c.Flows)
	}
	if totalFlows == 0 {
		t.Errorf("expected at least some sampled flows")
	}
	if totalFlows == 100 {
		t.Log("all flows sampled (unlikely but possible with random sampling)")
	}
}

func TestClassifierSamplingAll(t *testing.T) {
	config := SamplingConfig{
		Rate:     1.0,
		Strategy: SamplingRandom,
	}
	classifier := NewClassifier(config)

	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp"}, DSCP: DSCPDefault},
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) != 1 {
		t.Errorf("expected 1 class for all sampled, got %d", len(classes))
	}
	if len(classes[0].Flows) != 1 {
		t.Errorf("expected 1 flow, got %d", len(classes[0].Flows))
	}
}

func TestClassifierSamplingNone(t *testing.T) {
	config := SamplingConfig{
		Rate:     0.0,
		Strategy: SamplingRandom,
	}
	classifier := NewClassifier(config)

	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp"}, DSCP: DSCPDefault},
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) != 0 {
		t.Errorf("expected 0 classes for no sampling, got %d", len(classes))
	}
}

func TestClassifierSortByBytes(t *testing.T) {
	classifier := NewClassifier(DefaultSamplingConfig())

	// Create flows with different DSCP values to get multiple classes
	flows := []SampledFlow{
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPDefault},
		{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.5"), DstIP: netip.MustParseAddr("10.0.0.6"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: DSCPEF},
	}

	classes := classifier.ClassifyFlows(flows)
	if len(classes) < 2 {
		t.Errorf("expected at least 2 classes, got %d", len(classes))
	}
	// First class should have more bytes (3x1500 vs 1x1500)
	if len(classes) >= 2 && classes[0].TotalBytes < classes[1].TotalBytes {
		t.Errorf("expected first class to have more bytes than second")
	}
}

func TestFlowEquivalenceClassKeyFromPacketClass(t *testing.T) {
	pc := model.PacketClass{
		PrefixClassID: 5,
		Protocol:      "udp",
		SrcPort:       model.ExactPort(53),
		DstPort:       model.ExactPort(53),
	}

	key := FlowEquivalenceClassKeyFromPacketClass(pc, DSCPCS6)
	if key.PrefixClassID != 5 {
		t.Errorf("expected PrefixClassID 5, got %d", key.PrefixClassID)
	}
	if key.Protocol != "udp" {
		t.Errorf("expected protocol udp, got %s", key.Protocol)
	}
	if key.DSCP != DSCPCS6 {
		t.Errorf("expected DSCP CS6 (%d), got %d", DSCPCS6, key.DSCP)
	}
}

func TestFlowEquivalenceClassKeyString(t *testing.T) {
	key := FlowEquivalenceClassKey{
		PrefixClassID: 1,
		Protocol:      "tcp",
		DSCP:          DSCPEF,
	}
	str := key.String()
	if str == "" {
		t.Errorf("expected non-empty string representation")
	}
}

func TestClassifierTopNSampling(t *testing.T) {
	config := SamplingConfig{
		Rate:     0.5,
		Strategy: SamplingTopNByBytes,
	}
	classifier := NewClassifier(config)

	flows := make([]SampledFlow, 100)
	for i := range flows {
		flows[i] = SampledFlow{
			Flow: Flow{
				SrcIP:    netip.MustParseAddr("10.0.0.1"),
				DstIP:    netip.MustParseAddr("10.0.0.2"),
				Protocol: "tcp",
				SrcPort:  uint16(10000 + i),
				DstPort:  80,
			},
			DSCP: DSCPDefault,
		}
	}

	classes := classifier.ClassifyFlows(flows)
	totalFlows := 0
	for _, c := range classes {
		totalFlows += len(c.Flows)
	}
	// With 100 flows and 0.5 rate, top N should sample 50 flows
	if totalFlows != 50 {
		t.Errorf("expected 50 sampled flows with top_n 50%%, got %d", totalFlows)
	}
}
