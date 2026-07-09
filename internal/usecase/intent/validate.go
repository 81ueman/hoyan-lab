package intent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var varRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// AggregateFunc represents a parsed aggregate function.
type AggregateFunc struct {
	Name  string // "count", "distCnt", "distVals"
	Field string // non-empty only for distCnt and distVals
}

// ParseAggregate parses an aggregate function string.
// Valid forms: "count()", "distCnt(field)", "distVals(field)".
func ParseAggregate(s string) (AggregateFunc, error) {
	s = strings.TrimSpace(s)
	idx := strings.Index(s, "(")
	if idx == -1 {
		return AggregateFunc{}, fmt.Errorf("invalid aggregate function %q: missing opening parenthesis", s)
	}
	name := strings.TrimSpace(s[:idx])
	if !strings.HasSuffix(s, ")") {
		return AggregateFunc{}, fmt.Errorf("invalid aggregate function %q: missing closing parenthesis", s)
	}
	field := strings.TrimSpace(s[idx+1 : len(s)-1])
	switch name {
	case "count", "distCnt", "distVals":
	default:
		return AggregateFunc{}, fmt.Errorf("unknown aggregate function %q", name)
	}
	if name == "count" && field != "" {
		return AggregateFunc{}, fmt.Errorf("count() takes no arguments")
	}
	if (name == "distCnt" || name == "distVals") && field == "" {
		return AggregateFunc{}, fmt.Errorf("%s() requires a field argument", name)
	}
	return AggregateFunc{Name: name, Field: field}, nil
}

// Validate validates a Document, checking version, intents, and all RCL expressions.
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

		// Validate RCL expression recursively
		if err := validateRCLExpr(in.RCL, path+".rcl", doc); err != nil {
			return err
		}

		// Validate scenario reference
		if in.Scenario != "" {
			if _, ok := doc.Scenarios[in.Scenario]; !ok {
				return fmt.Errorf("%s.scenario: unknown scenario %q", path, in.Scenario)
			}
			scenario := doc.Scenarios[in.Scenario]
			if _, ok := doc.Snapshots[scenario.Snapshot]; !ok {
				return fmt.Errorf("%s.scenario: scenario %q references unknown snapshot %q", path, in.Scenario, scenario.Snapshot)
			}
		}

		// Validate Forall variables: each must be a single ${var} reference to doc.Vars
		forallVarNames := make(map[string]bool, len(in.Forall))
		for key, raw := range in.Forall {
			ref, ok := singleVarRef(raw)
			if !ok {
				return fmt.Errorf("%s.forall.%s: value must be a variable reference", path, key)
			}
			if _, ok := doc.Vars[ref]; !ok {
				return fmt.Errorf("%s.forall.%s: undefined var %q", path, key, ref)
			}
			forallVarNames[key] = true
		}

		// Validate variable references in RCL expression
		if err := validateRefsRCL(path+".rcl", in.RCL, doc.Vars, forallVarNames); err != nil {
			return err
		}
	}
	return nil
}

// validateRCLExpr recursively validates an RCL expression tree.
// Must be kept in sync with collectRefsInRCLExpr — every node type handled here
// should also be handled there for variable reference collection.
func validateRCLExpr(expr *RCLExpr, path string, doc *Document) error {
	if expr == nil {
		return fmt.Errorf("%s: nil expression", path)
	}
	switch {
	case expr.Guard != nil:
		return validateGuardExpr(expr.Guard, path+".guard", doc)

	case expr.Forall != nil:
		return validateForallExpr(expr.Forall, path+".forall", doc)

	case len(expr.And) > 0:
		if len(expr.And) < 2 {
			return fmt.Errorf("%s.and: must have at least 2 elements", path)
		}
		for j := range expr.And {
			if err := validateRCLExpr(&expr.And[j], fmt.Sprintf("%s.and[%d]", path, j), doc); err != nil {
				return err
			}
		}
		return nil

	case len(expr.Or) > 0:
		if len(expr.Or) < 2 {
			return fmt.Errorf("%s.or: must have at least 2 elements", path)
		}
		for j := range expr.Or {
			if err := validateRCLExpr(&expr.Or[j], fmt.Sprintf("%s.or[%d]", path, j), doc); err != nil {
				return err
			}
		}
		return nil

	case expr.Not != nil:
		if err := validateRCLExpr(expr.Not, path+".not", doc); err != nil {
			return err
		}
		return nil

	case expr.Imply != [2]*RCLExpr{}: // YAML populates Imply as a 2-element array; zero-value check detects "set or not"
		if expr.Imply[0] == nil || expr.Imply[1] == nil {
			return fmt.Errorf("%s.imply: must have exactly 2 sub-expressions", path)
		}
		if err := validateRCLExpr(expr.Imply[0], path+".imply[0]", doc); err != nil {
			return err
		}
		if err := validateRCLExpr(expr.Imply[1], path+".imply[1]", doc); err != nil {
			return err
		}
		return nil

	case expr.RIBEq != nil:
		return validateRIBEqExpr(expr.RIBEq, path+".rib_eq", doc)

	case expr.RIBEval != nil:
		return validateRIBEvalExpr(expr.RIBEval, path+".rib_eval", doc)

	case expr.PacketReachable != nil:
		return validatePacketReachableExpr(expr.PacketReachable, path+".packet_reachable")

	default:
		return fmt.Errorf("%s: empty expression (no fields set)", path)
	}
}

func validateGuardExpr(g *GuardExpr, path string, doc *Document) error {
	if g.Where == nil {
		return fmt.Errorf("%s.where: required", path)
	}
	return validateRCLExpr(&g.Intent, path+".intent", doc)
}

func validateForallExpr(f *ForallExpr, path string, doc *Document) error {
	if strings.TrimSpace(f.Var) == "" {
		return fmt.Errorf("%s.var: required", path)
	}
	return validateRCLExpr(&f.Intent, path+".intent", doc)
}

func validateRIBEqExpr(e *RIBEqExpr, path string, doc *Document) error {
	if e.Left == "" {
		return fmt.Errorf("%s.left: required", path)
	}
	if e.Right == "" {
		return fmt.Errorf("%s.right: required", path)
	}
	if _, ok := doc.Snapshots[e.Left]; !ok {
		return fmt.Errorf("%s.left: unknown snapshot %q", path, e.Left)
	}
	if _, ok := doc.Snapshots[e.Right]; !ok {
		return fmt.Errorf("%s.right: unknown snapshot %q", path, e.Right)
	}
	return nil
}

func validateRIBEvalExpr(e *RIBEvalExpr, path string, doc *Document) error {
	if e.Aggregate == "" {
		return fmt.Errorf("%s.aggregate: required", path)
	}
	agg, err := ParseAggregate(e.Aggregate)
	if err != nil {
		return fmt.Errorf("%s.aggregate: %v", path, err)
	}

	// Optional snapshot reference
	if e.Snapshot != "" {
		if _, ok := doc.Snapshots[e.Snapshot]; !ok {
			return fmt.Errorf("%s.snapshot: unknown snapshot %q", path, e.Snapshot)
		}
	}

	// At least one comparison operator must be specified
	if e.Eq == nil && e.Ne == nil && e.Gt == nil && e.Gte == nil && e.Lt == nil && e.Lte == nil {
		return fmt.Errorf("%s: at least one comparison operator (eq, ne, gt, gte, lt, lte) is required", path)
	}

	// Aggregate-specific type checks on comparison values.
	// Note: Gt/Gte/Lt/Lte are *int — their type already guarantees numeric values,
	// so only Eq and Ne need runtime type checking.
	switch agg.Name {
	case "count":
		for j, v := range e.Eq {
			if !isNumeric(v) {
				return fmt.Errorf("%s.eq[%d]: count() comparison requires numeric value, got %T", path, j, v)
			}
		}
		for j, v := range e.Ne {
			if !isNumeric(v) {
				return fmt.Errorf("%s.ne[%d]: count() comparison requires numeric value, got %T", path, j, v)
			}
		}
	case "distVals":
		for j, v := range e.Eq {
			if _, ok := v.([]any); !ok {
				return fmt.Errorf("%s.eq[%d]: distVals() comparison requires array value, got %T", path, j, v)
			}
		}
		for j, v := range e.Ne {
			if _, ok := v.([]any); !ok {
				return fmt.Errorf("%s.ne[%d]: distVals() comparison requires array value, got %T", path, j, v)
			}
		}
	}
	return nil
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	// YAML may deserialize numbers as strings in some contexts
	if s, ok := v.(string); ok {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return true
		}
	}
	return false
}

func validatePacketReachableExpr(e *PacketReachableExpr, path string) error {
	if e.From == "" {
		return fmt.Errorf("%s.from: required", path)
	}
	if e.To == "" {
		return fmt.Errorf("%s.to: required", path)
	}
	switch e.Protocol {
	case "icmp", "tcp", "udp":
	default:
		return fmt.Errorf("%s.protocol: unsupported protocol %q", path, e.Protocol)
	}
	if e.DstPort < 0 || e.DstPort > 65535 {
		return fmt.Errorf("%s.dst_port: out of range (0-65535)", path)
	}
	return nil
}

// --- Variable reference checking ---

// singleVarRef checks if raw is a string consisting entirely of a single ${var} reference.
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

func validateRefsRCL(path string, expr *RCLExpr, vars map[string]any, localVars map[string]bool) error {
	refs := collectRefsInRCLExpr(expr)
	for _, ref := range refs {
		if localVars[ref] {
			continue
		}
		if _, ok := vars[ref]; !ok {
			return fmt.Errorf("%s: undefined var %q", path, ref)
		}
	}
	return nil
}

// collectRefsInRCLExpr walks the expression tree and collects all ${var} references.
// Must be kept in sync with validateRCLExpr — every node type handled there
// should also be handled here so variable references are not missed.
func collectRefsInRCLExpr(expr *RCLExpr) []string {
	if expr == nil {
		return nil
	}
	var refs []string
	switch {
	case expr.Guard != nil:
		refs = append(refs, refsInAny(expr.Guard.Where)...)
		refs = append(refs, collectRefsInRCLExpr(&expr.Guard.Intent)...)
	case expr.Forall != nil:
		refs = append(refs, collectRefsInRCLExpr(&expr.Forall.Intent)...)
	case len(expr.And) > 0:
		for i := range expr.And {
			refs = append(refs, collectRefsInRCLExpr(&expr.And[i])...)
		}
	case len(expr.Or) > 0:
		for i := range expr.Or {
			refs = append(refs, collectRefsInRCLExpr(&expr.Or[i])...)
		}
	case expr.Not != nil:
		refs = append(refs, collectRefsInRCLExpr(expr.Not)...)
	case expr.Imply != [2]*RCLExpr{}: // zero-value check: if Imply field was set in YAML
		if expr.Imply[0] != nil {
			refs = append(refs, collectRefsInRCLExpr(expr.Imply[0])...)
		}
		if expr.Imply[1] != nil {
			refs = append(refs, collectRefsInRCLExpr(expr.Imply[1])...)
		}
	case expr.RIBEq != nil:
		refs = append(refs, refsInAny(expr.RIBEq.Where)...)
	case expr.RIBEval != nil:
		refs = append(refs, refsInAny(expr.RIBEval.Where)...)
	case expr.PacketReachable != nil:
		refs = append(refs, refsInString(expr.PacketReachable.From)...)
		refs = append(refs, refsInString(expr.PacketReachable.VRF)...)
		refs = append(refs, refsInString(expr.PacketReachable.To)...)
	}
	return refs
}

func refsInString(s string) []string {
	var refs []string
	for _, m := range varRefRE.FindAllStringSubmatch(s, -1) {
		refs = append(refs, m[1])
	}
	return refs
}

func refsInAny(v any) []string {
	var refs []string
	switch x := v.(type) {
	case string:
		refs = append(refs, refsInString(x)...)
	case map[string]any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	case []any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	}
	return refs
}
