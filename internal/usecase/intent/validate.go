package intent

import (
	"fmt"
	"strings"
)

func Validate(doc *Document) error {
	if doc.Version != "hoyan/v1" {
		return fmt.Errorf("version: unsupported or missing version %q", doc.Version)
	}
	for i, in := range doc.Intents {
		path := fmt.Sprintf("intents[%d]", i)
		if strings.TrimSpace(in.Name) == "" {
			return fmt.Errorf("%s.name: required", path)
		}
		if in.RCL == nil {
			return fmt.Errorf("%s.rcl: required", path)
		}
	}
	return nil
}
