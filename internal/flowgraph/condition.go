package flowgraph

import (
	"fmt"
	"maps"
	"strings"

	"github.com/expr-lang/expr"
)

// EvaluateCondition compiles and runs a boolean expression via
// expr-lang/expr against vars, used by the chatbot and IVR condition nodes.
// Keys of vars are top-level identifiers; unknown identifiers resolve to nil,
// so `status == "active"` is false when status isn't set rather than an
// error. The result is coerced to bool — non-bool truthy values count as true.
func EvaluateCondition(expression string, vars map[string]any) (bool, error) {
	env := make(map[string]any, len(vars))
	maps.Copy(env, vars)

	program, err := expr.Compile(expression, expr.Env(env), expr.AllowUndefinedVariables())
	if err != nil {
		return false, fmt.Errorf("compile: %w", err)
	}
	out, err := expr.Run(program, env)
	if err != nil {
		return false, fmt.Errorf("run: %w", err)
	}
	switch v := out.(type) {
	case bool:
		return v, nil
	case nil:
		return false, nil
	case string:
		return v != "" && strings.ToLower(v) != "false", nil
	case float64:
		return v != 0, nil
	case int:
		return v != 0, nil
	}
	// Anything else (slices, maps) counts as truthy if non-nil.
	return out != nil, nil
}
