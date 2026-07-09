package intent

import (
	"fmt"
)

// evalRCLExpr recursively evaluates an RCL expression and returns the status
// ("pass" or "fail") along with the actual measurement data. The rowFilter
// parameter accumulates where predicates from enclosing Guard expressions.
func evalRCLExpr(expr *RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	if expr == nil {
		return "fail", Actual{Reason: "nil expression"}
	}

	switch {
	case expr.Guard != nil:
		return evalGuard(expr.Guard, snapshot, rowFilter, scenario, ctx)
	case expr.Forall != nil:
		return evalForall(expr.Forall, snapshot, rowFilter, scenario, ctx)
	case len(expr.And) > 0:
		return evalAnd(expr.And, snapshot, rowFilter, scenario, ctx)
	case len(expr.Or) > 0:
		return evalOr(expr.Or, snapshot, rowFilter, scenario, ctx)
	case expr.Not != nil:
		innerStatus, innerActual := evalRCLExpr(expr.Not, snapshot, rowFilter, scenario, ctx)
		if innerStatus == "pass" {
			return "fail", innerActual
		}
		return "pass", innerActual
	case expr.Imply != [2]*RCLExpr{}:
		return evalImply(expr.Imply, snapshot, rowFilter, scenario, ctx)
	case expr.RIBEq != nil:
		return evalRIBEq(expr.RIBEq, snapshot, rowFilter, scenario, ctx)
	case expr.RIBEval != nil:
		return evalRIBEval(expr.RIBEval, snapshot, rowFilter, scenario, ctx)
	case expr.PacketReachable != nil:
		return evalPacketReachable(expr.PacketReachable, snapshot, scenario)
	default:
		return "fail", Actual{Reason: "empty expression"}
	}
}

// ---------------------------------------------------------------------------
// And / Or
// ---------------------------------------------------------------------------

func evalAnd(exprs []RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	var actuals []Actual
	for i := range exprs {
		status, a := evalRCLExpr(&exprs[i], snapshot, rowFilter, scenario, ctx)
		actuals = append(actuals, a)
		if status == "fail" {
			return "fail", combineActuals(actuals)
		}
	}
	if len(actuals) > 0 {
		return "pass", combineActuals(actuals)
	}
	return "pass", Actual{Count: 0}
}

func evalOr(exprs []RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	var actuals []Actual
	for i := range exprs {
		status, a := evalRCLExpr(&exprs[i], snapshot, rowFilter, scenario, ctx)
		actuals = append(actuals, a)
		if status == "pass" {
			return "pass", combineActuals(actuals)
		}
	}
	return "fail", combineActuals(actuals)
}

// ---------------------------------------------------------------------------
// Imply
// ---------------------------------------------------------------------------

func evalImply(pair [2]*RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	if pair[0] == nil || pair[1] == nil {
		return "fail", Actual{Reason: "imply requires exactly 2 sub-expressions"}
	}
	// Evaluate antecedent
	antStatus, _ := evalRCLExpr(pair[0], snapshot, rowFilter, scenario, ctx)
	if antStatus == "fail" {
		// Antecedent false → implication is vacuously true
		return "pass", Actual{Count: 0, Reason: "antecedent is false"}
	}
	// Antecedent true → evaluate consequent; propagate full consequent Actual
	conStatus, conActual := evalRCLExpr(pair[1], snapshot, rowFilter, scenario, ctx)
	conActual.Reason = fmt.Sprintf("antecedent passed, consequent: %s", conStatus)
	return conStatus, conActual
}
