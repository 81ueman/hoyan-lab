package intent

func Expand(doc *Document) (*ExpandedDocument, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return &ExpandedDocument{
		Version:   doc.Version,
		Snapshots: doc.Snapshots,
		Scenarios: doc.Scenarios,
		Intents:   doc.Intents,
	}, nil
}
