package verify

import (
	"fmt"
	"sort"
	"strings"
)

func collapseResults(results []Result) []Result {
	type aggregate struct {
		result Result
		seen   map[string]bool
	}
	groups := map[string]*aggregate{}
	var order []string
	for _, result := range results {
		key := collapseKey(result)
		group, ok := groups[key]
		if !ok {
			cp := result
			cp.PrefixClass = &PrefixClassMetadata{}
			group = &aggregate{result: cp, seen: map[string]bool{}}
			groups[key] = group
			order = append(order, key)
		}
		if result.PrefixClass != nil && result.PrefixClass.ClassID != nil {
			classKey := fmt.Sprintf("%d", *result.PrefixClass.ClassID)
			if !group.seen[classKey] {
				group.seen[classKey] = true
				group.result.PrefixClass.ClassIDs = append(group.result.PrefixClass.ClassIDs, *result.PrefixClass.ClassID)
			}
		}
		if result.PrefixClass != nil {
			if result.PrefixClass.Space != "" && !containsString(group.result.PrefixClass.Spaces, result.PrefixClass.Space) {
				group.result.PrefixClass.Spaces = append(group.result.PrefixClass.Spaces, result.PrefixClass.Space)
			}
			for _, predicate := range result.PrefixClass.MatchedPredicates {
				if !containsString(group.result.PrefixClass.MatchedPredicates, predicate) {
					group.result.PrefixClass.MatchedPredicates = append(group.result.PrefixClass.MatchedPredicates, predicate)
				}
			}
		}
	}
	out := make([]Result, 0, len(order))
	for _, key := range order {
		result := groups[key].result
		if result.PrefixClass != nil {
			sort.Slice(result.PrefixClass.ClassIDs, func(i, j int) bool {
				return result.PrefixClass.ClassIDs[i] < result.PrefixClass.ClassIDs[j]
			})
			sort.Strings(result.PrefixClass.Spaces)
			sort.Strings(result.PrefixClass.MatchedPredicates)
		}
		out = append(out, result)
	}
	return out
}

func collapseKey(result Result) string {
	return strings.Join([]string{
		result.Name,
		string(result.Type),
		fmt.Sprint(result.Metadata.Reachable),
		fmt.Sprint(result.Metadata.Expected),
		strings.Join(result.Counterexample(), ","),
		result.Metadata.Reason,
		result.ReachableCondition(),
		result.UnreachableCondition(),
	}, "\x00")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
