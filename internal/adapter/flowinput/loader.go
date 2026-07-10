package flowinput

import (
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// rawFlow is the JSON representation of a traffic flow.
type rawFlow struct {
	SrcIP   string `json:"src_ip"`
	DstIP   string `json:"dst_ip"`
	Proto   string `json:"protocol"`
	SrcPort int    `json:"src_port"`
	DstPort int    `json:"dst_port"`
	Bytes   uint64 `json:"bytes"`
	Ingress string `json:"ingress"`
}

// LoadJSON reads flow data from a JSON stream and returns LocatedFlows.
func LoadJSON(r io.Reader) ([]model.LocatedFlow, error) {
	var raws []rawFlow
	if err := json.NewDecoder(r).Decode(&raws); err != nil {
		return nil, fmt.Errorf("decoding flow JSON: %w", err)
	}

	flows := make([]model.LocatedFlow, 0, len(raws))
	for i, raw := range raws {
		srcIP, err := netip.ParseAddr(raw.SrcIP)
		if err != nil {
			return nil, fmt.Errorf("flow[%d]: invalid src_ip %q: %w", i, raw.SrcIP, err)
		}
		dstIP, err := netip.ParseAddr(raw.DstIP)
		if err != nil {
			return nil, fmt.Errorf("flow[%d]: invalid dst_ip %q: %w", i, raw.DstIP, err)
		}

		// Parse ingress node and optional interface
		ingressNode, ingressIntf := SplitIngress(raw.Ingress)

		flows = append(flows, model.LocatedFlow{
			Flow: model.Flow{
				SrcIP:    srcIP,
				DstIP:    dstIP,
				Protocol: raw.Proto,
				SrcPort:  raw.SrcPort,
				DstPort:  raw.DstPort,
			},
			IngressNode: ingressNode,
			IngressIntf: ingressIntf,
			Bytes:       raw.Bytes,
		})
	}

	return flows, nil
}

// LoadJSONFile reads flow data from a JSON file path.
func LoadJSONFile(path string) ([]model.LocatedFlow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening flow file %q: %w", path, err)
	}
	defer f.Close()
	return LoadJSON(f)
}

// SplitIngress splits an ingress identifier into node and interface.
// Supports formats like "node" or "node:interface".
func SplitIngress(ingress string) (node, intf string) {
	if ingress == "" {
		return "", ""
	}
	for i := 0; i < len(ingress); i++ {
		if ingress[i] == ':' {
			return ingress[:i], ingress[i+1:]
		}
	}
	return ingress, ""
}
