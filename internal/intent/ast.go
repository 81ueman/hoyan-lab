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
}

type Assertion struct {
	Exists *bool       `yaml:"exists,omitempty" json:"exists,omitempty"`
	Count  *CountCheck `yaml:"count,omitempty" json:"count,omitempty"`
}

type CountCheck struct {
	GTE    *int `yaml:"gte,omitempty" json:"gte,omitempty"`
	Equals *int `yaml:"equals,omitempty" json:"equals,omitempty"`
}

type ExpandedDocument struct {
	Version   string              `json:"version"`
	Snapshots map[string]Snapshot `json:"snapshots,omitempty"`
	Scenarios map[string]Scenario `json:"scenarios,omitempty"`
	Intents   []Intent            `json:"intents"`
}
