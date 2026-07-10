package traffic

import (
	"math"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
)

// KFailFinding represents a k-failure finding: a link that exceeds the
// utilization threshold with k additional failures.
type KFailFinding struct {
	LinkName       string
	UtilizationPct float64
	K              int
	Failures       []solver.FailureElement
}

// KFailResult holds the result of a k-failure tolerance analysis.
type KFailResult struct {
	Findings []KFailFinding
}

// KFailAnalyzer analyzes k-failure tolerance for traffic links.
// For each link, it finds the minimum number of additional failures
// that would cause overload on any link.
type KFailAnalyzer struct {
	config SimulatorConfig
}

// NewKFailAnalyzer creates a new KFailAnalyzer.
func NewKFailAnalyzer(config SimulatorConfig) *KFailAnalyzer {
	return &KFailAnalyzer{config: config}
}

// Analyze analyzes k-failure tolerance for all links in the topology.
//
// rootNode is the ingress node for traffic simulation.
// thresholdPct is the utilization percentage threshold (e.g., 80 = 80%).
// maxK is the maximum number of simultaneous failures to search.
//
// It uses failure.SearchElements and failure.FindElementCombo to enumerate
// failure combinations and find which ones cause link overload.
func (ka *KFailAnalyzer) Analyze(
	rootNode string,
	topo *model.Topology,
	fibs FIBTable,
	ecs []FlowEquivalenceClass,
	thresholdPct float64,
	maxK int,
) *KFailResult {
	if rootNode == "" {
		return &KFailResult{}
	}

	cache := NewTDGCache()
	ws := NewWhatIfSimulator(ka.config)

	// Compute base (no-failure) link loads
	baseResult := ws.Simulate(rootNode, failure.None(), ecs, cache, fibs)
	if baseResult == nil {
		return &KFailResult{}
	}

	// Check for base overload (k=0)
	var findings []KFailFinding
	for linkName, ll := range baseResult.LinkLoads {
		bw := linkBandwidthForName(topo, linkName)
		if bw == 0 {
			continue
		}
		utilPct := float64(ll.Bytes) / float64(bw) * 100.0
		if utilPct >= thresholdPct {
			findings = append(findings, KFailFinding{
				LinkName:       linkName,
				UtilizationPct: math.Round(utilPct*100) / 100,
				K:              0,
				Failures:       nil,
			})
		}
	}

	// If maxK is 0, only check base overload
	if maxK <= 0 {
		sortFindings(findings)
		return &KFailResult{Findings: findings}
	}

	// Get failure search elements
	elements := failure.SearchElements(topo, failure.SearchOptions{
		IncludeLinks: true,
		IncludeNodes: true,
		MaxFailures:  maxK,
	})

	if len(elements) == 0 {
		sortFindings(findings)
		return &KFailResult{Findings: findings}
	}

	// For each k from 1 to maxK, search for failure combinations that cause overload
	for k := 1; k <= maxK; k++ {
		if k > len(elements) {
			break
		}

		failure.FindElementCombo(elements, k, 0, nil, func(combo []solver.FailureElement) bool {
			// Build failure set from combo
			failSet := failure.SetFromElements(combo)

			// Simulate with this failure combination
			result := ws.Simulate(rootNode, failSet, ecs, cache, fibs)
			if result == nil {
				return false
			}

			// Check all links for overload
			for linkName, ll := range result.LinkLoads {
				bw := linkBandwidthForName(topo, linkName)
				if bw == 0 {
					continue
				}
				utilPct := float64(ll.Bytes) / float64(bw) * 100.0
				if utilPct >= thresholdPct {
					// Check if this link is already in findings (don't add duplicates with higher k)
					alreadyFound := false
					for _, f := range findings {
						if f.LinkName == linkName {
							alreadyFound = true
							break
						}
					}
					if !alreadyFound {
						comboCopy := make([]solver.FailureElement, len(combo))
						copy(comboCopy, combo)
						findings = append(findings, KFailFinding{
							LinkName:       linkName,
							UtilizationPct: math.Round(utilPct*100) / 100,
							K:              k,
							Failures:       comboCopy,
						})
					}
				}
			}

			return false // continue searching
		})
	}

	sortFindings(findings)
	return &KFailResult{Findings: findings}
}

// linkBandwidthForName looks up a link's bandwidth in the topology.
// It first tries a direct match on link.Name. If no match is found,
// it parses the from->to format (used by TDG traversal) and matches
// by endpoint node names using link.A and link.B.
func linkBandwidthForName(topo *model.Topology, linkName string) uint64 {
	// Direct match on link.Name (used when topology link names match the
	// from->to convention, e.g. test topologies)
	for _, link := range topo.Links {
		if link.Name == linkName {
			return linkBandwidth(link)
		}
	}

	// Parse the from->to format used by TDG traversal and match by
	// endpoint nodes. This handles real topology link names like
	// "r1-eth1__r2-eth1" which don't match the "from->to" naming.
	if parts := strings.SplitN(linkName, "->", 2); len(parts) == 2 {
		from, to := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		for _, link := range topo.Links {
			if (link.A == from && link.B == to) || (link.A == to && link.B == from) {
				return linkBandwidth(link)
			}
		}
	}

	return 0
}

func sortFindings(findings []KFailFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].K != findings[j].K {
			return findings[i].K < findings[j].K
		}
		return findings[i].LinkName < findings[j].LinkName
	})
}
