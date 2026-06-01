package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func writeFormatJSONOnly(out io.Writer, format string, value any) error {
	if format != "json" {
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q", "json")}
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writePrefixUniverseStats(out io.Writer, stats model.PrefixUniverseStats) {
	fmt.Fprintf(out, "predicates=%d unique=%d classes=%d build=%s max_class_cidrs=%d\n",
		stats.PredicateCount,
		stats.UniquePredicateCount,
		stats.ClassCount,
		stats.BuildDuration,
		stats.MaxClassCIDRs,
	)
	if len(stats.PredicateSources) == 0 {
		return
	}
	fmt.Fprintln(out, "sources:")
	categories := make([]string, 0, len(stats.PredicateSources))
	for category := range stats.PredicateSources {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	for _, category := range categories {
		fmt.Fprintf(out, "  %s: %d\n", category, stats.PredicateSources[category])
	}
}
