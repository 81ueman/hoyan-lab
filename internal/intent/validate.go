package intent

import (
	"fmt"
	"regexp"
	"strings"
)

var varRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func Validate(doc *Document) error {
	if doc.Version != "hoyan/v1" {
		return fmt.Errorf("version: unsupported or missing version %q", doc.Version)
	}
	for i, in := range doc.Intents {
		path := fmt.Sprintf("intents[%d]", i)
		if strings.TrimSpace(in.Name) == "" {
			return fmt.Errorf("%s.name: required", path)
		}
		if in.Check.Compare != nil {
			if err := validateCompare(path, in.Check.Compare, doc); err != nil {
				return err
			}
		} else {
			switch in.Check.Table {
			case "rib", "fib":
			default:
				return fmt.Errorf("%s.check.table: unsupported table %q", path, in.Check.Table)
			}
			if _, ok := doc.Scenarios[in.Check.Scenario]; !ok {
				return fmt.Errorf("%s.check.scenario: unknown scenario %q", path, in.Check.Scenario)
			}
			scenario := doc.Scenarios[in.Check.Scenario]
			if _, ok := doc.Snapshots[scenario.Snapshot]; !ok {
				return fmt.Errorf("%s.check.scenario: scenario %q references unknown snapshot %q", path, in.Check.Scenario, scenario.Snapshot)
			}
		}
		assertion := effectiveAssertion(in)
		if in.Check.Compare == nil && assertion.Exists == nil && assertion.Count == nil && assertion.DistinctCount == nil && assertion.DistinctValues == nil {
			return fmt.Errorf("%s.assert: exists or count is required", path)
		}
		if err := validateRefs(path, in, doc.Vars); err != nil {
			return err
		}
	}
	return nil
}

func validateCompare(path string, compare *CompareCheck, doc *Document) error {
	switch compare.Table {
	case "rib":
	default:
		return fmt.Errorf("%s.check.compare.table: unsupported table %q", path, compare.Table)
	}
	if compare.Relation != "equal" {
		return fmt.Errorf("%s.check.compare.relation: unsupported relation %q", path, compare.Relation)
	}
	if _, ok := doc.Snapshots[compare.Left.Snapshot]; !ok {
		return fmt.Errorf("%s.check.compare.left.snapshot: unknown snapshot %q", path, compare.Left.Snapshot)
	}
	if _, ok := doc.Snapshots[compare.Right.Snapshot]; !ok {
		return fmt.Errorf("%s.check.compare.right.snapshot: unknown snapshot %q", path, compare.Right.Snapshot)
	}
	return nil
}

func validateRefs(path string, in Intent, vars map[string]any) error {
	forallVars := map[string]bool{}
	for key, raw := range in.Forall {
		forallVars[key] = true
		ref, ok := singleVarRef(raw)
		if !ok {
			return fmt.Errorf("%s.forall.%s: value must be a variable reference", path, key)
		}
		value, ok := vars[ref]
		if !ok {
			return fmt.Errorf("%s.forall.%s: undefined var %q", path, key, ref)
		}
		if _, ok := toStringSlice(value); !ok {
			return fmt.Errorf("%s.forall.%s: var %q must be a list", path, key, ref)
		}
	}
	for _, ref := range append(refsInAny(in.Check), refsInAny(effectiveAssertion(in))...) {
		if forallVars[ref] {
			continue
		}
		if _, ok := vars[ref]; !ok {
			return fmt.Errorf("%s.check: undefined var %q", path, ref)
		}
	}
	return nil
}

func singleVarRef(raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	m := varRefRE.FindStringSubmatch(s)
	if len(m) != 2 || m[0] != s {
		return "", false
	}
	return m[1], true
}

func refsInAny(v any) []string {
	var refs []string
	switch x := v.(type) {
	case string:
		for _, m := range varRefRE.FindAllStringSubmatch(x, -1) {
			refs = append(refs, m[1])
		}
	case map[string]any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	case []any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	case Check:
		refs = append(refs, refsInAny(x.Where)...)
		refs = append(refs, refsInAny(x.Assert)...)
		if x.Compare != nil {
			refs = append(refs, refsInAny(x.Compare.Left.Where)...)
			refs = append(refs, refsInAny(x.Compare.Right.Where)...)
		}
	case Assertion:
		if x.DistinctValues != nil {
			refs = append(refs, refsInAny(x.DistinctValues.Equals)...)
		}
	}
	return refs
}

func effectiveAssertion(in Intent) Assertion {
	if in.Check.Assert.Exists != nil || in.Check.Assert.Count != nil || in.Check.Assert.DistinctCount != nil || in.Check.Assert.DistinctValues != nil || in.Check.Assert.Relation != "" {
		return in.Check.Assert
	}
	return in.Assert
}
