package intentdsl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
)

// parser implements a recursive-descent parser for the Hoyan DSL.
type parser struct {
	lex  *lexer
	tok  token // current lookahead token
	vars map[string]any
}

func newParser(lex *lexer) *parser {
	p := &parser{lex: lex}
	p.next() // prime the lookahead
	return p
}

func (p *parser) next() {
	p.tok = p.lex.next()
}

// pos returns the current token's position.
func (p *parser) pos() string {
	return fmt.Sprintf("%s:%d:%d", p.lex.s.Filename, p.tok.pos.Line, p.tok.pos.Column)
}

// errorf creates a parse error with position info.
func (p *parser) errorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %s", p.pos(), msg)
}

// expect consumes the current token if kind matches, else returns error.
func (p *parser) expect(kind tokenKind) error {
	if p.tok.kind != kind {
		return p.errorf("expected %s, got %s (%q)", kind, p.tok.kind, p.tok.text)
	}
	p.next()
	return nil
}

// ParseDocument parses a complete Hoyan DSL document.
func (p *parser) ParseDocument() (*intent.Document, error) {
	doc := &intent.Document{
		Vars:      make(map[string]any),
		Snapshots: make(map[string]intent.Snapshot),
		Scenarios: make(map[string]intent.Scenario),
	}
	p.vars = doc.Vars

	// version = "..."
	if p.tok.kind == tokKeywordVersion {
		if err := p.parseVersion(doc); err != nil {
			return nil, err
		}
	}

	// var declarations, snapshots, scenarios, intents in any order
	for p.tok.kind != tokEOF {
		switch p.tok.kind {
		case tokKeywordLet:
			if err := p.parseVarDecl(doc); err != nil {
				return nil, err
			}
		case tokKeywordSnapshot:
			if err := p.parseSnapshotDecl(doc); err != nil {
				return nil, err
			}
		case tokKeywordScenario:
			if err := p.parseScenarioDecl(doc); err != nil {
				return nil, err
			}
		case tokKeywordIntent:
			intent, err := p.parseIntentDecl()
			if err != nil {
				return nil, err
			}
			doc.Intents = append(doc.Intents, intent)
		default:
			return nil, p.errorf("unexpected token %s (%q)", p.tok.kind, p.tok.text)
		}
	}

	return doc, nil
}

func (p *parser) parseVersion(doc *intent.Document) error {
	p.next() // consume 'version'
	if err := p.expect(tokAssign); err != nil {
		return err
	}
	if p.tok.kind != tokString {
		return p.errorf("expected string for version, got %s (%q)", p.tok.kind, p.tok.text)
	}
	doc.Version = p.tok.text
	p.next()
	return nil
}

func (p *parser) parseVarDecl(doc *intent.Document) error {
	p.next() // consume 'let'
	if p.tok.kind != tokIdent {
		return p.errorf("expected identifier after 'let', got %s (%q)", p.tok.kind, p.tok.text)
	}
	name := p.tok.text
	p.next()

	if err := p.expect(tokAssign); err != nil {
		return err
	}

	val, err := p.parseValue()
	if err != nil {
		return err
	}
	doc.Vars[name] = val
	return nil
}

func (p *parser) parseSnapshotDecl(doc *intent.Document) error {
	p.next() // consume 'snapshot'
	if p.tok.kind != tokString {
		return p.errorf("expected snapshot name (string), got %s (%q)", p.tok.kind, p.tok.text)
	}
	name := p.tok.text
	p.next()

	if err := p.expect(tokLBrace); err != nil {
		return err
	}

	snap := intent.Snapshot{}

	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		switch p.tok.kind {
		case tokIdent, tokKeywordLab:
			if p.tok.text != "lab" {
				return p.errorf("unexpected field %q in snapshot body", p.tok.text)
			}
			p.next()
			if err := p.expect(tokAssign); err != nil {
				return err
			}
			if p.tok.kind != tokString {
				return p.errorf("expected string for lab, got %s (%q)", p.tok.kind, p.tok.text)
			}
			snap.Lab = p.tok.text
			p.next()
		default:
			return p.errorf("unexpected token in snapshot body: %s (%q)", p.tok.kind, p.tok.text)
		}
	}

	if err := p.expect(tokRBrace); err != nil {
		return err
	}

	doc.Snapshots[name] = snap
	return nil
}

func (p *parser) parseScenarioDecl(doc *intent.Document) error {
	p.next() // consume 'scenario'
	if p.tok.kind != tokString {
		return p.errorf("expected scenario name (string), got %s (%q)", p.tok.kind, p.tok.text)
	}
	name := p.tok.text
	p.next()

	if err := p.expect(tokLBrace); err != nil {
		return err
	}

	sc := intent.Scenario{}

	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		switch p.tok.kind {
		case tokIdent, tokKeywordSnapshot:
			// Could be 'snapshot' as identifier rather than keyword
			if p.tok.text == "snapshot" {
				p.next()
				if err := p.expect(tokAssign); err != nil {
					return err
				}
				if p.tok.kind != tokString {
					return p.errorf("expected string for snapshot ref, got %s (%q)", p.tok.kind, p.tok.text)
				}
				sc.Snapshot = p.tok.text
				p.next()
				// optional trailing comma
				if p.tok.kind == tokComma {
					p.next()
				}
			} else {
				return p.errorf("unexpected identifier %q in scenario body", p.tok.text)
			}
		case tokKeywordFailures:
			failures, err := p.parseFailureBlock()
			if err != nil {
				return err
			}
			sc.Failures = failures
		default:
			return p.errorf("unexpected token in scenario body: %s (%q)", p.tok.kind, p.tok.text)
		}
	}

	if err := p.expect(tokRBrace); err != nil {
		return err
	}

	doc.Scenarios[name] = sc
	return nil
}

func (p *parser) parseFailureBlock() (intent.FailureConstraints, error) {
	p.next() // consume 'failures'
	if err := p.expect(tokLBrace); err != nil {
		return intent.FailureConstraints{}, err
	}

	fc := intent.FailureConstraints{}

	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		switch p.tok.kind {
		case tokKeywordMax:
			p.next()
			if err := p.expect(tokAssign); err != nil {
				return fc, err
			}
			if p.tok.kind != tokInt {
				return fc, p.errorf("expected int for max, got %s (%q)", p.tok.kind, p.tok.text)
			}
			fc.Max, _ = strconv.Atoi(p.tok.text)
			p.next()
			if p.tok.kind == tokComma {
				p.next()
			}
		case tokKeywordIncludeLinkRoles,
			tokKeywordExcludeLinkRoles,
			tokKeywordIncludeLinks,
			tokKeywordExcludeLinks,
			tokKeywordIncludeNodeRoles,
			tokKeywordExcludeNodeRoles,
			tokKeywordIncludeNodes,
			tokKeywordExcludeNodes:
			kind := p.tok.kind
			p.next()
			if err := p.expect(tokAssign); err != nil {
				return fc, err
			}
			val, err := p.parseValue()
			if err != nil {
				return fc, err
			}
			p.setFailureArrayField(&fc, kind, toStringSlice(val))
			if p.tok.kind == tokComma {
				p.next()
			}
		default:
			return fc, p.errorf("unexpected token in failures block: %s (%q)", p.tok.kind, p.tok.text)
		}
	}

	if err := p.expect(tokRBrace); err != nil {
		return fc, err
	}

	return fc, nil
}

// setFailureArrayField assigns the parsed string slice to the correct
// FailureConstraints field based on the keyword token kind.
func (p *parser) setFailureArrayField(fc *intent.FailureConstraints, kind tokenKind, vals []string) {
	switch kind {
	case tokKeywordIncludeLinkRoles:
		fc.IncludeLinkRoles = vals
	case tokKeywordExcludeLinkRoles:
		fc.ExcludeLinkRoles = vals
	case tokKeywordIncludeLinks:
		fc.IncludeLinks = vals
	case tokKeywordExcludeLinks:
		fc.ExcludeLinks = vals
	case tokKeywordIncludeNodeRoles:
		fc.IncludeNodeRoles = vals
	case tokKeywordExcludeNodeRoles:
		fc.ExcludeNodeRoles = vals
	case tokKeywordIncludeNodes:
		fc.IncludeNodes = vals
	case tokKeywordExcludeNodes:
		fc.ExcludeNodes = vals
	}
}

func (p *parser) parseIntentDecl() (intent.Intent, error) {
	p.next() // consume 'intent'
	if p.tok.kind != tokString {
		return intent.Intent{}, p.errorf("expected intent name (string), got %s (%q)", p.tok.kind, p.tok.text)
	}
	name := p.tok.text
	p.next()

	if err := p.expect(tokLBrace); err != nil {
		return intent.Intent{}, err
	}

	in := intent.Intent{Name: name}

	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		switch {
		case (p.tok.kind == tokIdent || p.tok.kind == tokKeywordScenario) && p.tok.text == "scenario":
			p.next()
			if err := p.expect(tokAssign); err != nil {
				return in, err
			}
			if p.tok.kind != tokString {
				return in, p.errorf("expected string for scenario, got %s (%q)", p.tok.kind, p.tok.text)
			}
			in.Scenario = p.tok.text
			p.next()
		case p.tok.kind == tokKeywordForall:
			forall, err := p.parseDocLevelForall()
			if err != nil {
				return in, err
			}
			in.Forall = forall
		default:
			// It's the RCL expression. IntentBody allows exactly one top-level RCL expression.
			if in.RCL != nil {
				return in, p.errorf("multiple top-level expressions in intent %q", in.Name)
			}
			expr, err := p.parseRCLExpr()
			if err != nil {
				return in, err
			}
			in.RCL = expr
		}
	}

	if err := p.expect(tokRBrace); err != nil {
		return in, err
	}

	return in, nil
}

// parseDocLevelForall parses the document-level forall (key=value pairs).
// e.g.: forall src in $customers
func (p *parser) parseDocLevelForall() (map[string]any, error) {
	p.next() // consume 'forall'
	if p.tok.kind != tokIdent {
		return nil, p.errorf("expected variable name after 'forall', got %s (%q)", p.tok.kind, p.tok.text)
	}
	varName := p.tok.text
	p.next()

	if err := p.expect(tokKeywordIn); err != nil {
		return nil, err
	}

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return map[string]any{varName: val}, nil
}

// parseRCLExpr parses an RCL-level expression.
func (p *parser) parseRCLExpr() (*intent.RCLExpr, error) {
	switch p.tok.kind {
	case tokKeywordWhen:
		return p.parseGuard()
	case tokKeywordFor:
		return p.parseForall()
	case tokKeywordRib:
		return p.parseRibEval()
	case tokKeywordPacket:
		return p.parsePacket()
	case tokKeywordRibEq:
		return p.parseRibEq()
	case tokKeywordAnd:
		return p.parseAnd()
	case tokKeywordOr:
		return p.parseOr()
	case tokKeywordNot:
		return p.parseNot()
	case tokKeywordIf:
		return p.parseImply()
	case tokKeywordCount, tokKeywordDistCnt, tokKeywordDistVals:
		// Bare aggregate expr (auto-detect: treated as rib_eval with no where)
		return p.parseBareAggregateExpr()
	case tokLBrace:
		// Bare block - try to parse as aggregate inside
		return p.parseBlockAsRibEval()
	default:
		return nil, p.errorf("unexpected token %s (%q) in RCL expression", p.tok.kind, p.tok.text)
	}
}

// parseGuard parses: when wherePredicates { expr }
func (p *parser) parseGuard() (*intent.RCLExpr, error) {
	p.next() // consume 'when'

	where, err := p.parseWherePredicates()
	if err != nil {
		return nil, err
	}

	// Parse the block as the inner intent
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	// The block content determines the inner intent type
	inner := p.blockToRCLExpr(block)

	return &intent.RCLExpr{
		Guard: &intent.GuardExpr{
			Where:  where,
			Intent: *inner,
		},
	}, nil
}

// parseForall parses: for var in array { expr }
func (p *parser) parseForall() (*intent.RCLExpr, error) {
	p.next() // consume 'for'

	if p.tok.kind != tokIdent {
		return nil, p.errorf("expected variable name after 'for', got %s (%q)", p.tok.kind, p.tok.text)
	}
	varName := p.tok.text
	p.next()

	if err := p.expect(tokKeywordIn); err != nil {
		return nil, err
	}

	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	vals, err := p.forallValues(val)
	if err != nil {
		return nil, err
	}

	// Parse the block as the inner intent
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	inner := p.blockToRCLExpr(block)
	if len(vals) > 0 && rclUsesVarRef(inner, varName) {
		exprs := make([]intent.RCLExpr, 0, len(vals))
		for _, v := range vals {
			expr := substituteRCLVarRef(inner, varName, v)
			exprs = append(exprs, expr)
		}
		return &intent.RCLExpr{And: exprs}, nil
	}

	return &intent.RCLExpr{
		Forall: &intent.ForallExpr{
			Var:    varName,
			In:     vals,
			Intent: *inner,
		},
	}, nil
}

func (p *parser) forallValues(val any) ([]string, error) {
	switch v := val.(type) {
	case []string:
		return v, nil
	case []any:
		return toStringSlice(v), nil
	case string:
		if ref, ok := parserSingleVarRef(v); ok {
			if raw, exists := p.vars[ref]; exists {
				return toStringSlice(raw), nil
			}
		}
		return []string{v}, nil
	default:
		return nil, p.errorf("expected array for 'in', got %T", val)
	}
}

func parserSingleVarRef(s string) (string, bool) {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") && len(s) > 3 {
		return strings.TrimSuffix(strings.TrimPrefix(s, "${"), "}"), true
	}
	return "", false
}

func rclUsesVarRef(expr *intent.RCLExpr, varName string) bool {
	ref := "${" + varName + "}"
	return rclContainsString(expr, ref)
}

func rclContainsString(expr *intent.RCLExpr, ref string) bool {
	if expr == nil {
		return false
	}
	switch {
	case expr.Guard != nil:
		return anyContainsString(expr.Guard.Where, ref) || rclContainsString(&expr.Guard.Intent, ref)
	case expr.Forall != nil:
		return anyContainsString(expr.Forall.Var, ref) || anyContainsString(expr.Forall.In, ref) || rclContainsString(&expr.Forall.Intent, ref)
	case len(expr.And) > 0:
		for i := range expr.And {
			if rclContainsString(&expr.And[i], ref) {
				return true
			}
		}
	case len(expr.Or) > 0:
		for i := range expr.Or {
			if rclContainsString(&expr.Or[i], ref) {
				return true
			}
		}
	case expr.Not != nil:
		return rclContainsString(expr.Not, ref)
	case expr.Imply[0] != nil || expr.Imply[1] != nil:
		return rclContainsString(expr.Imply[0], ref) || rclContainsString(expr.Imply[1], ref)
	case expr.RIBEq != nil:
		return anyContainsString(expr.RIBEq.Where, ref)
	case expr.RIBEval != nil:
		return anyContainsString(expr.RIBEval.Where, ref) || anyContainsString(expr.RIBEval.Eq, ref) || anyContainsString(expr.RIBEval.Ne, ref)
	case expr.PacketReachable != nil:
		return anyContainsString(expr.PacketReachable.From, ref) || anyContainsString(expr.PacketReachable.To, ref) || anyContainsString(expr.PacketReachable.VRF, ref)
	}
	return false
}

func anyContainsString(v any, ref string) bool {
	switch x := v.(type) {
	case string:
		return x == ref
	case []string:
		for _, item := range x {
			if item == ref {
				return true
			}
		}
	case []any:
		for _, item := range x {
			if anyContainsString(item, ref) {
				return true
			}
		}
	case map[string]any:
		for _, item := range x {
			if anyContainsString(item, ref) {
				return true
			}
		}
	}
	return false
}

func substituteRCLVarRef(expr *intent.RCLExpr, varName, value string) intent.RCLExpr {
	if expr == nil {
		return intent.RCLExpr{}
	}
	ref := "${" + varName + "}"
	out := *expr
	switch {
	case expr.Guard != nil:
		inner := substituteRCLVarRef(&expr.Guard.Intent, varName, value)
		out.Guard = &intent.GuardExpr{Where: substituteAnyVarRef(expr.Guard.Where, ref, value).(map[string]any), Intent: inner}
	case expr.Forall != nil:
		inner := substituteRCLVarRef(&expr.Forall.Intent, varName, value)
		out.Forall = &intent.ForallExpr{Var: substituteStringVarRef(expr.Forall.Var, ref, value), In: substituteStringSliceVarRef(expr.Forall.In, ref, value), Intent: inner}
	case len(expr.And) > 0:
		out.And = make([]intent.RCLExpr, len(expr.And))
		for i := range expr.And {
			out.And[i] = substituteRCLVarRef(&expr.And[i], varName, value)
		}
	case len(expr.Or) > 0:
		out.Or = make([]intent.RCLExpr, len(expr.Or))
		for i := range expr.Or {
			out.Or[i] = substituteRCLVarRef(&expr.Or[i], varName, value)
		}
	case expr.Not != nil:
		inner := substituteRCLVarRef(expr.Not, varName, value)
		out.Not = &inner
	case expr.Imply[0] != nil || expr.Imply[1] != nil:
		left := substituteRCLVarRef(expr.Imply[0], varName, value)
		right := substituteRCLVarRef(expr.Imply[1], varName, value)
		out.Imply = [2]*intent.RCLExpr{&left, &right}
	case expr.RIBEq != nil:
		out.RIBEq = &intent.RIBEqExpr{Left: expr.RIBEq.Left, Right: expr.RIBEq.Right, Where: substituteAnyVarRef(expr.RIBEq.Where, ref, value).(map[string]any)}
	case expr.RIBEval != nil:
		out.RIBEval = &intent.RIBEvalExpr{Snapshot: expr.RIBEval.Snapshot, Where: substituteAnyVarRef(expr.RIBEval.Where, ref, value).(map[string]any), Aggregate: expr.RIBEval.Aggregate, Eq: substituteAnySliceVarRef(expr.RIBEval.Eq, ref, value), Ne: substituteAnySliceVarRef(expr.RIBEval.Ne, ref, value), Gt: expr.RIBEval.Gt, Gte: expr.RIBEval.Gte, Lt: expr.RIBEval.Lt, Lte: expr.RIBEval.Lte}
	case expr.PacketReachable != nil:
		out.PacketReachable = &intent.PacketReachableExpr{From: substituteStringVarRef(expr.PacketReachable.From, ref, value), VRF: substituteStringVarRef(expr.PacketReachable.VRF, ref, value), To: substituteStringVarRef(expr.PacketReachable.To, ref, value), Protocol: expr.PacketReachable.Protocol, DstPort: expr.PacketReachable.DstPort, Expect: expr.PacketReachable.Expect}
	}
	return out
}

func substituteAnyVarRef(v any, ref, value string) any {
	switch x := v.(type) {
	case nil:
		return map[string]any(nil)
	case string:
		return substituteStringVarRef(x, ref, value)
	case []string:
		return substituteStringSliceVarRef(x, ref, value)
	case []any:
		return substituteAnySliceVarRef(x, ref, value)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = substituteAnyVarRef(item, ref, value)
		}
		return out
	default:
		return v
	}
}

func substituteStringVarRef(s, ref, value string) string {
	if s == ref {
		return value
	}
	return s
}

func substituteStringSliceVarRef(in []string, ref, value string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, item := range in {
		out[i] = substituteStringVarRef(item, ref, value)
	}
	return out
}

func substituteAnySliceVarRef(in []any, ref, value string) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, len(in))
	for i, item := range in {
		out[i] = substituteAnyVarRef(item, ref, value)
	}
	return out
}

// parseRibEval parses: rib [where ...] { aggregate }
// or: rib where ... { aggregate }
func (p *parser) parseRibEval() (*intent.RCLExpr, error) {
	p.next() // consume 'rib'

	var where map[string]any

	// Optional where clause
	if p.tok.kind == tokKeywordWhere {
		var err error
		where, err = p.parseWhereClause()
		if err != nil {
			return nil, err
		}
	}

	// Parse block with aggregate
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	agg, err := p.blockToAggregate(block)
	if err != nil {
		return nil, err
	}

	ribEval := &intent.RIBEvalExpr{
		Where:     where,
		Aggregate: agg.aggregate,
		Eq:        agg.eq,
		Ne:        agg.ne,
		Gt:        agg.gt,
		Gte:       agg.gte,
		Lt:        agg.lt,
		Lte:       agg.lte,
	}

	return &intent.RCLExpr{
		RIBEval: ribEval,
	}, nil
}

// parseBareAggregateExpr handles a bare aggregate like count() >= 4
// This auto-detects as rib_eval with no where clause.
func (p *parser) parseBareAggregateExpr() (*intent.RCLExpr, error) {
	agg, err := p.parseAggregateExpr()
	if err != nil {
		return nil, err
	}

	ribEval := &intent.RIBEvalExpr{
		Aggregate: agg.aggregate,
		Eq:        agg.eq,
		Ne:        agg.ne,
		Gt:        agg.gt,
		Gte:       agg.gte,
		Lt:        agg.lt,
		Lte:       agg.lte,
	}

	return &intent.RCLExpr{
		RIBEval: ribEval,
	}, nil
}

// parseBlockAsRibEval handles a bare block { ... } as rib_eval.
func (p *parser) parseBlockAsRibEval() (*intent.RCLExpr, error) {
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	agg, err := p.blockToAggregate(block)
	if err != nil {
		return nil, err
	}

	ribEval := &intent.RIBEvalExpr{
		Aggregate: agg.aggregate,
		Eq:        agg.eq,
		Ne:        agg.ne,
		Gt:        agg.gt,
		Gte:       agg.gte,
		Lt:        agg.lt,
		Lte:       agg.lte,
	}

	return &intent.RCLExpr{
		RIBEval: ribEval,
	}, nil
}

// parsePacket parses: packet from ... to ... tcp/80 expect true
func (p *parser) parsePacket() (*intent.RCLExpr, error) {
	p.next() // consume 'packet'

	if err := p.expect(tokKeywordFrom); err != nil {
		return nil, err
	}

	from, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	if err := p.expect(tokKeywordTo); err != nil {
		return nil, err
	}

	to, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// protocol: icmp, tcp/N, udp/N
	var protocol string
	var dstPort int

	switch p.tok.kind {
	case tokKeywordIcmp:
		protocol = "icmp"
		p.next()
	case tokKeywordTcp:
		protocol = "tcp"
		p.next()
		if err := p.expect(tokSlash); err != nil {
			return nil, err
		}
		if p.tok.kind != tokInt {
			return nil, p.errorf("expected port number after tcp/, got %s (%q)", p.tok.kind, p.tok.text)
		}
		dstPort, _ = strconv.Atoi(p.tok.text)
		p.next()
	case tokKeywordUdp:
		protocol = "udp"
		p.next()
		if err := p.expect(tokSlash); err != nil {
			return nil, err
		}
		if p.tok.kind != tokInt {
			return nil, p.errorf("expected port number after udp/, got %s (%q)", p.tok.kind, p.tok.text)
		}
		dstPort, _ = strconv.Atoi(p.tok.text)
		p.next()
	default:
		return nil, p.errorf("expected protocol (icmp/tcp/udp), got %s (%q)", p.tok.kind, p.tok.text)
	}

	// optional vrf
	var vrf string
	if p.tok.kind == tokKeywordVrf {
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		vrf = stringValue(val)
	}

	if err := p.expect(tokKeywordExpect); err != nil {
		return nil, err
	}

	if p.tok.kind != tokBool {
		return nil, p.errorf("expected bool for expect, got %s (%q)", p.tok.kind, p.tok.text)
	}
	expect := p.tok.text == "true"
	p.next()

	return &intent.RCLExpr{
		PacketReachable: &intent.PacketReachableExpr{
			From:     stringValue(from),
			To:       stringValue(to),
			Protocol: protocol,
			DstPort:  dstPort,
			VRF:      vrf,
			Expect:   expect,
		},
	}, nil
}

// parseRibEq parses: rib_eq left = "..." right = "..." where ...
func (p *parser) parseRibEq() (*intent.RCLExpr, error) {
	p.next() // consume 'rib_eq'

	if err := p.expect(tokKeywordLeft); err != nil {
		return nil, err
	}
	if err := p.expect(tokAssign); err != nil {
		return nil, err
	}
	if p.tok.kind != tokString {
		return nil, p.errorf("expected string for left, got %s (%q)", p.tok.kind, p.tok.text)
	}
	left := p.tok.text
	p.next()

	if err := p.expect(tokKeywordRight); err != nil {
		return nil, err
	}
	if err := p.expect(tokAssign); err != nil {
		return nil, err
	}
	if p.tok.kind != tokString {
		return nil, p.errorf("expected string for right, got %s (%q)", p.tok.kind, p.tok.text)
	}
	right := p.tok.text
	p.next()

	var where map[string]any
	if p.tok.kind == tokKeywordWhere {
		var err error
		where, err = p.parseWhereClause()
		if err != nil {
			return nil, err
		}
	}

	return &intent.RCLExpr{
		RIBEq: &intent.RIBEqExpr{
			Left:  left,
			Right: right,
			Where: where,
		},
	}, nil
}

// parseAnd parses: and { expr+ }
func (p *parser) parseAnd() (*intent.RCLExpr, error) {
	return p.parseCombinator("and")
}

// parseOr parses: or { expr+ }
func (p *parser) parseOr() (*intent.RCLExpr, error) {
	return p.parseCombinator("or")
}

// parseCombinator parses an and{...} or or{...} block, consuming the keyword.
func (p *parser) parseCombinator(kw string) (*intent.RCLExpr, error) {
	p.next() // consume 'and' or 'or'
	if err := p.expect(tokLBrace); err != nil {
		return nil, err
	}

	var exprs []intent.RCLExpr
	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		expr, err := p.parseRCLExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, *expr)
	}

	if err := p.expect(tokRBrace); err != nil {
		return nil, err
	}

	if kw == "and" {
		return &intent.RCLExpr{And: exprs}, nil
	}
	return &intent.RCLExpr{Or: exprs}, nil
}

// parseNot parses: not expr
func (p *parser) parseNot() (*intent.RCLExpr, error) {
	p.next() // consume 'not'

	// 'not' can be followed by a block { ... } or a bare expression
	var inner *intent.RCLExpr
	if p.tok.kind == tokLBrace {
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		inner = p.blockToRCLExpr(block)
	} else {
		var err error
		inner, err = p.parseRCLExpr()
		if err != nil {
			return nil, err
		}
	}

	return &intent.RCLExpr{Not: inner}, nil
}

// parseImply parses: if expr then expr
func (p *parser) parseImply() (*intent.RCLExpr, error) {
	p.next() // consume 'if'

	var antecedent, consequent *intent.RCLExpr

	if p.tok.kind == tokLBrace {
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		antecedent = p.blockToRCLExpr(block)
	} else {
		var err error
		antecedent, err = p.parseRCLExpr()
		if err != nil {
			return nil, err
		}
	}

	if err := p.expect(tokKeywordThen); err != nil {
		return nil, err
	}

	if p.tok.kind == tokLBrace {
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		consequent = p.blockToRCLExpr(block)
	} else {
		var err error
		consequent, err = p.parseRCLExpr()
		if err != nil {
			return nil, err
		}
	}

	return &intent.RCLExpr{Imply: [2]*intent.RCLExpr{antecedent, consequent}}, nil
}

// parseBlock parses: { ... } and returns a list of RCLExprs inside.
func (p *parser) parseBlock() ([]*intent.RCLExpr, error) {
	if err := p.expect(tokLBrace); err != nil {
		return nil, err
	}

	var exprs []*intent.RCLExpr
	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		expr, err := p.parseRCLExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}

	if err := p.expect(tokRBrace); err != nil {
		return nil, err
	}
	if len(exprs) == 0 {
		return nil, p.errorf("empty block")
	}

	return exprs, nil
}

// blockToRCLExpr converts a block (list of parsed RCLExprs) to a single RCLExpr.
// A single-element block returns that element directly.
// Multiple elements are implicitly AND-combined (wrapped in an And expression).
func (p *parser) blockToRCLExpr(block []*intent.RCLExpr) *intent.RCLExpr {
	if len(block) == 1 {
		return block[0]
	}

	exprs := make([]intent.RCLExpr, len(block))
	for i, e := range block {
		exprs[i] = *e
	}
	return &intent.RCLExpr{And: exprs}
}

// aggregateResult holds parsed aggregate expression data.
type aggregateResult struct {
	aggregate string
	eq        []any
	ne        []any
	gt        *int
	gte       *int
	lt        *int
	lte       *int
}

// blockToAggregate extracts an aggregate expression from a block.
// The block should contain exactly one aggregate expression.
func (p *parser) blockToAggregate(block []*intent.RCLExpr) (*aggregateResult, error) {
	if len(block) != 1 {
		return nil, p.errorf("rib block must contain exactly one aggregate expression")
	}
	if agg := extractAggregate(block[0]); agg != nil {
		return agg, nil
	}
	return nil, p.errorf("expected aggregate expression in block")
}

// extractAggregate tries to extract aggregate info from an RCLExpr.
func extractAggregate(expr *intent.RCLExpr) *aggregateResult {
	if expr.RIBEval != nil {
		re := expr.RIBEval
		return &aggregateResult{
			aggregate: re.Aggregate,
			eq:        re.Eq,
			ne:        re.Ne,
			gt:        re.Gt,
			gte:       re.Gte,
			lt:        re.Lt,
			lte:       re.Lte,
		}
	}
	return nil
}

// parseAggregateExpr parses: count() >= 4, distCnt(nexthop) >= 2, etc.
func (p *parser) parseAggregateExpr() (*aggregateResult, error) {
	var aggName string

	switch p.tok.kind {
	case tokKeywordCount:
		aggName = "count()"
		p.next()
	case tokKeywordDistCnt:
		aggName = "distCnt(" + p.tok.text + ")"
		p.next()
	case tokKeywordDistVals:
		aggName = "distVals(" + p.tok.text + ")"
		p.next()
	default:
		return nil, p.errorf("expected aggregate function, got %s (%q)", p.tok.kind, p.tok.text)
	}

	result := &aggregateResult{aggregate: aggName}

	// Parse comparison operator
	switch p.tok.kind {
	case tokGte:
		p.next()
		val, err := p.parseAggregateValue()
		if err != nil {
			return nil, err
		}
		result.gte = intPtr(val)
	case tokGt:
		p.next()
		val, err := p.parseAggregateValue()
		if err != nil {
			return nil, err
		}
		result.gt = intPtr(val)
	case tokLte:
		p.next()
		val, err := p.parseAggregateValue()
		if err != nil {
			return nil, err
		}
		result.lte = intPtr(val)
	case tokLt:
		p.next()
		val, err := p.parseAggregateValue()
		if err != nil {
			return nil, err
		}
		result.lt = intPtr(val)
	case tokEq:
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		// For eq/ne, values can be arrays or scalars
		switch v := val.(type) {
		case []any:
			result.eq = v
		case string, int, float64, bool:
			result.eq = []any{v}
		default:
			return nil, p.errorf("unexpected value type for ==: %T", val)
		}
	case tokNe:
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		switch v := val.(type) {
		case []any:
			result.ne = v
		case string, int, float64, bool:
			result.ne = []any{v}
		default:
			return nil, p.errorf("unexpected value type for !=: %T", val)
		}
	default:
		return nil, p.errorf("expected comparison operator, got %s (%q)", p.tok.kind, p.tok.text)
	}

	return result, nil
}

// parseAggregateValue parses an int literal for aggregate comparison.
func (p *parser) parseAggregateValue() (int, error) {
	if p.tok.kind != tokInt {
		return 0, p.errorf("expected integer, got %s (%q)", p.tok.kind, p.tok.text)
	}
	val, _ := strconv.Atoi(p.tok.text)
	p.next()
	return val, nil
}

// parseWhereClause parses: where wherePredicates
func (p *parser) parseWhereClause() (map[string]any, error) {
	p.next() // consume 'where'
	return p.parseWherePredicates()
}

// parseOneWhereEntry parses exactly one where predicate entry.
// It handles simple predicates (key = value) and compound predicates (and, or, not, imply).
func (p *parser) parseOneWhereEntry() (map[string]any, error) {
	switch p.tok.kind {
	case tokKeywordAnd:
		p.next()
		if err := p.expect(tokLBrace); err != nil {
			return nil, err
		}
		var andList []any
		for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
			pred, err := p.parseOneWhereEntry()
			if err != nil {
				return nil, err
			}
			andList = append(andList, pred)
		}
		if err := p.expect(tokRBrace); err != nil {
			return nil, err
		}
		return map[string]any{"and": andList}, nil

	case tokKeywordOr:
		p.next()
		if err := p.expect(tokLBrace); err != nil {
			return nil, err
		}
		var orList []any
		for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
			pred, err := p.parseOneWhereEntry()
			if err != nil {
				return nil, err
			}
			orList = append(orList, pred)
		}
		if err := p.expect(tokRBrace); err != nil {
			return nil, err
		}
		return map[string]any{"or": orList}, nil

	case tokKeywordImply, tokKeywordIf:
		p.next()
		if err := p.expect(tokLBrace); err != nil {
			return nil, err
		}
		antecedent, err := p.parseOneWhereEntry()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokKeywordThen); err != nil {
			return nil, err
		}
		consequent, err := p.parseOneWhereEntry()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokRBrace); err != nil {
			return nil, err
		}
		return map[string]any{"imply": []any{antecedent, consequent}}, nil

	case tokKeywordNot:
		p.next()
		if p.tok.kind == tokLBrace {
			p.next()
			inner, err := p.parseOneWhereEntry()
			if err != nil {
				return nil, err
			}
			if err := p.expect(tokRBrace); err != nil {
				return nil, err
			}
			return map[string]any{"not": inner}, nil
		}
		inner, err := p.parseSingleWherePredicate()
		if err != nil {
			return nil, err
		}
		return map[string]any{"not": inner}, nil

	default:
		return p.parseSingleWherePredicate()
	}
}

// parseWherePredicates parses one or more where predicates, comma-separated.
func (p *parser) parseWherePredicates() (map[string]any, error) {
	result := make(map[string]any)
	first := true

	for {
		if !first && p.tok.kind == tokComma {
			p.next()
		}

		// Check for termination: we're done if next token is not a predicate start.
		// A where/when clause itself must contain at least one predicate; only a
		// trailing comma after an already parsed predicate may terminate here.
		if !p.isWherePredicateStart() {
			if first {
				return nil, p.errorf("expected where predicate")
			}
			break
		}
		first = false

		// Handle compound where predicates: and { ... }, or { ... }, imply { ... }, not
		switch p.tok.kind {
		case tokKeywordAnd:
			p.next()
			if err := p.expect(tokLBrace); err != nil {
				return nil, err
			}
			var andList []any
			for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
				pred, err := p.parseOneWhereEntry()
				if err != nil {
					return nil, err
				}
				andList = append(andList, pred)
			}
			if err := p.expect(tokRBrace); err != nil {
				return nil, err
			}
			result["and"] = andList
			return result, nil

		case tokKeywordOr:
			p.next()
			if err := p.expect(tokLBrace); err != nil {
				return nil, err
			}
			var orList []any
			for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
				pred, err := p.parseOneWhereEntry()
				if err != nil {
					return nil, err
				}
				orList = append(orList, pred)
			}
			if err := p.expect(tokRBrace); err != nil {
				return nil, err
			}
			result["or"] = orList
			return result, nil

		case tokKeywordImply:
			fallthrough
		case tokKeywordIf:
			// imply { pred1 then pred2 }
			p.next()
			if err := p.expect(tokLBrace); err != nil {
				return nil, err
			}
			antecedent, err := p.parseWherePredicates()
			if err != nil {
				return nil, err
			}
			if err := p.expect(tokKeywordThen); err != nil {
				return nil, err
			}
			consequent, err := p.parseWherePredicates()
			if err != nil {
				return nil, err
			}
			if err := p.expect(tokRBrace); err != nil {
				return nil, err
			}
			result["imply"] = []any{antecedent, consequent}
			return result, nil

		case tokKeywordNot:
			p.next()
			// not prefix = "..." or not { prefix = "..." }
			if p.tok.kind == tokLBrace {
				p.next()
				inner, err := p.parseWherePredicates()
				if err != nil {
					return nil, err
				}
				if err := p.expect(tokRBrace); err != nil {
					return nil, err
				}
				result["not"] = inner
			} else {
				inner, err := p.parseSingleWherePredicate()
				if err != nil {
					return nil, err
				}
				result["not"] = inner
			}
			// Don't return — parse more predicates if comma follows
			continue

		default:
			// Simple predicate: ident = value, ident contains value, etc.
			inner, err := p.parseSingleWherePredicate()
			if err != nil {
				return nil, err
			}
			// Merge into result
			for k, v := range inner {
				result[k] = v
			}
		}
	}

	return result, nil
}

// isWherePredicateStart checks if the current token can start a where predicate.
func (p *parser) isWherePredicateStart() bool {
	switch p.tok.kind {
	case tokIdent, tokKeywordAnd, tokKeywordOr, tokKeywordNot, tokKeywordImply, tokKeywordIf:
		return true
	default:
		return false
	}
}

// parseSingleWherePredicate parses one where predicate like: ident = value, ident contains value, etc.
func (p *parser) parseSingleWherePredicate() (map[string]any, error) {
	if p.tok.kind != tokIdent {
		return nil, p.errorf("expected identifier for where predicate, got %s (%q)", p.tok.kind, p.tok.text)
	}
	key := p.tok.text
	p.next()

	switch p.tok.kind {
	case tokAssign, tokEq:
		// ident = value or ident == value (exact match)
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return map[string]any{key: val}, nil

	case tokNe:
		// ident != value; emit the existing evaluator's negated-predicate shape.
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return map[string]any{"not": map[string]any{key: val}}, nil

	case tokKeywordContains:
		// ident contains value
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return map[string]any{key: map[string]any{"contains": val}}, nil

	case tokKeywordMatches:
		// ident matches value
		p.next()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return map[string]any{key: map[string]any{"matches": val}}, nil

	case tokKeywordWithin:
		// prefix within value; emit the existing evaluator's prefix_within key.
		p.next()
		if key != "prefix" {
			return nil, p.errorf("within is only supported for prefix predicates")
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return map[string]any{"prefix_within": val}, nil

	default:
		return nil, p.errorf("expected =, !=, contains, matches, or within after %q, got %s (%q)", key, p.tok.kind, p.tok.text)
	}
}

// parseValue parses a value: string, int, float, bool, array, or varref.
func (p *parser) parseValue() (any, error) {
	switch p.tok.kind {
	case tokString:
		val := p.tok.text
		p.next()
		return val, nil
	case tokInt:
		val, _ := strconv.Atoi(p.tok.text)
		p.next()
		return val, nil
	case tokFloat:
		val, _ := strconv.ParseFloat(p.tok.text, 64)
		p.next()
		return val, nil
	case tokBool:
		val := p.tok.text == "true"
		p.next()
		return val, nil
	case tokVarRef:
		val := "${" + p.tok.text + "}" // wrap as ${var_name} for YAML compatibility
		p.next()
		return val, nil
	case tokLBrack:
		return p.parseArray()
	default:
		return nil, p.errorf("expected value, got %s (%q)", p.tok.kind, p.tok.text)
	}
}

// parseArray parses: [ value, value, ... ]
func (p *parser) parseArray() ([]any, error) {
	p.next() // consume '['

	var vals []any

	for p.tok.kind != tokRBrack && p.tok.kind != tokEOF {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		vals = append(vals, val)

		if p.tok.kind == tokComma {
			p.next()
			// trailing comma allowed
			if p.tok.kind == tokRBrack {
				break
			}
		}
	}

	if err := p.expect(tokRBrack); err != nil {
		return nil, err
	}

	return vals, nil
}

// stringValue converts an any value to string.
func stringValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprint(v)
	}
}

// toStringSlice converts an any value to []string.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case []any:
		result := make([]string, len(val))
		for i, iv := range val {
			result[i] = fmt.Sprint(iv)
		}
		return result
	case []string:
		return val
	case string:
		return []string{val}
	default:
		return nil
	}
}

func intPtr(v int) *int {
	return &v
}
