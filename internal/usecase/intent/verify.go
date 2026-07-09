package intent

import "fmt"

func Verify(doc *Document) (Report, error) {
	return VerifyWithProvider(doc, DefaultSnapshotProvider{})
}

func VerifyWithProvider(doc *Document, provider SnapshotProvider) (Report, error) {
	if provider == nil {
		return Report{}, fmt.Errorf("snapshot provider is nil")
	}
	_, err := Expand(doc)
	if err != nil {
		return Report{}, err
	}
	return Report{Version: "hoyan.intent.report/v1"}, nil
}
