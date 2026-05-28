package intent

type Report struct {
	Version string        `json:"version"`
	Summary ReportSummary `json:"summary"`
	Results []Result      `json:"results"`
}

type ReportSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type Result struct {
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	Table           string         `json:"table"`
	Scenario        string         `json:"scenario"`
	Snapshot        string         `json:"snapshot"`
	Group           map[string]any `json:"group"`
	Assertion       Assertion      `json:"assertion"`
	Actual          Actual         `json:"actual"`
	Counterexamples []any          `json:"counterexamples"`
}

type Actual struct {
	Count         int      `json:"count"`
	Reachable     *bool    `json:"reachable,omitempty"`
	DistinctCount int      `json:"distinct_count,omitempty"`
	Values        []any    `json:"values,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Path          []string `json:"path,omitempty"`
	Rows          []string `json:"rows,omitempty"`
	AddedRows     []any    `json:"added_rows,omitempty"`
	RemovedRows   []any    `json:"removed_rows,omitempty"`
	ChangedRows   []any    `json:"changed_rows,omitempty"`
}

type FailureCounterexample struct {
	FailedLinks []string `json:"failed_links,omitempty"`
	FailedNodes []string `json:"failed_nodes,omitempty"`
	Path        []string `json:"path,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}
