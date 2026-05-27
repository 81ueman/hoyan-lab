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
	Counterexamples []string       `json:"counterexamples"`
}

type Actual struct {
	Count  int      `json:"count"`
	Reason string   `json:"reason,omitempty"`
	Rows   []string `json:"rows,omitempty"`
}
