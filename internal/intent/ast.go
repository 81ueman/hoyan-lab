package intent

type Document struct {
	Version   string              `yaml:"version" json:"version"`
	Vars      map[string]any      `yaml:"vars,omitempty" json:"vars,omitempty"`
	Snapshots map[string]Snapshot `yaml:"snapshots,omitempty" json:"snapshots,omitempty"`
	Scenarios map[string]Scenario `yaml:"scenarios,omitempty" json:"scenarios,omitempty"`
	Intents   []Intent            `yaml:"intents,omitempty" json:"intents,omitempty"`
}

type Snapshot struct {
	Lab string `yaml:"lab" json:"lab"`
}

type Scenario struct {
	Snapshot string `yaml:"snapshot" json:"snapshot"`
}

type Intent struct {
	Name   string         `yaml:"name" json:"name"`
	Forall map[string]any `yaml:"forall,omitempty" json:"forall,omitempty"`
	Check  Check          `yaml:"check" json:"check"`
	Assert Assertion      `yaml:"assert" json:"assert"`
	Group  map[string]any `yaml:"-" json:"group,omitempty"`
}

type Check struct {
	Table    string         `yaml:"table" json:"table"`
	Scenario string         `yaml:"scenario" json:"scenario"`
	Where    map[string]any `yaml:"where,omitempty" json:"where,omitempty"`
	GroupBy  []string       `yaml:"group_by,omitempty" json:"group_by,omitempty"`
	Assert   Assertion      `yaml:"assert,omitempty" json:"assert,omitempty"`
	Compare  *CompareCheck  `yaml:"compare,omitempty" json:"compare,omitempty"`
}

type Assertion struct {
	Exists         *bool                `yaml:"exists,omitempty" json:"exists,omitempty"`
	Count          *CountCheck          `yaml:"count,omitempty" json:"count,omitempty"`
	DistinctCount  *DistinctCountCheck  `yaml:"distinct_count,omitempty" json:"distinct_count,omitempty"`
	DistinctValues *DistinctValuesCheck `yaml:"distinct_values,omitempty" json:"distinct_values,omitempty"`
	Relation       string               `yaml:"relation,omitempty" json:"relation,omitempty"`
}

type CountCheck struct {
	GTE    *int `yaml:"gte,omitempty" json:"gte,omitempty"`
	Equals *int `yaml:"equals,omitempty" json:"equals,omitempty"`
}

type DistinctCountCheck struct {
	Field  string `yaml:"field" json:"field"`
	GTE    *int   `yaml:"gte,omitempty" json:"gte,omitempty"`
	Equals *int   `yaml:"equals,omitempty" json:"equals,omitempty"`
}

type DistinctValuesCheck struct {
	Field  string `yaml:"field" json:"field"`
	Equals []any  `yaml:"equals" json:"equals"`
}

type CompareCheck struct {
	Table    string      `yaml:"table" json:"table"`
	Left     CompareSide `yaml:"left" json:"left"`
	Right    CompareSide `yaml:"right" json:"right"`
	Relation string      `yaml:"relation" json:"relation"`
}

type CompareSide struct {
	Snapshot string         `yaml:"snapshot" json:"snapshot"`
	Where    map[string]any `yaml:"where,omitempty" json:"where,omitempty"`
}

type ExpandedDocument struct {
	Version   string              `json:"version"`
	Snapshots map[string]Snapshot `json:"snapshots,omitempty"`
	Scenarios map[string]Scenario `json:"scenarios,omitempty"`
	Intents   []Intent            `json:"intents"`
}
