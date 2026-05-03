package mesahub

import (
	"fmt"
	"strings"
)

// WhereClause maps column names to filter values or operator objects.
//
// Shorthand equality:
//
//	WhereClause{"id": 1}
//
// Operator object:
//
//	WhereClause{"age": map[string]any{"gte": 18}, "name": map[string]any{"like": "%alice%"}}
type WhereClause = map[string]any

// BuildWhere compiles a WhereClause into a parameterised SQL fragment and bindings.
// Returns ("", nil) when clause is empty or nil.
func BuildWhere(clause WhereClause) (string, []any) {
	if len(clause) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(clause))
	bindings := make([]any, 0)

	for key, op := range clause {
		col := quoteIdent(key)

		if op == nil {
			continue
		}

		opMap, isMap := op.(map[string]any)
		if !isMap {
			parts = append(parts, fmt.Sprintf("%s = ?", col))
			bindings = append(bindings, op)
			continue
		}

		switch {
		case hasKey(opMap, "is_null"):
			parts = append(parts, fmt.Sprintf("%s IS NULL", col))
		case hasKey(opMap, "is_not_null"):
			parts = append(parts, fmt.Sprintf("%s IS NOT NULL", col))
		case hasKey(opMap, "eq"):
			parts = append(parts, fmt.Sprintf("%s = ?", col))
			bindings = append(bindings, opMap["eq"])
		case hasKey(opMap, "ne"):
			parts = append(parts, fmt.Sprintf("%s != ?", col))
			bindings = append(bindings, opMap["ne"])
		case hasKey(opMap, "gt"):
			parts = append(parts, fmt.Sprintf("%s > ?", col))
			bindings = append(bindings, opMap["gt"])
		case hasKey(opMap, "gte"):
			parts = append(parts, fmt.Sprintf("%s >= ?", col))
			bindings = append(bindings, opMap["gte"])
		case hasKey(opMap, "lt"):
			parts = append(parts, fmt.Sprintf("%s < ?", col))
			bindings = append(bindings, opMap["lt"])
		case hasKey(opMap, "lte"):
			parts = append(parts, fmt.Sprintf("%s <= ?", col))
			bindings = append(bindings, opMap["lte"])
		case hasKey(opMap, "like"):
			parts = append(parts, fmt.Sprintf("%s LIKE ?", col))
			bindings = append(bindings, opMap["like"])
		case hasKey(opMap, "not_like"):
			parts = append(parts, fmt.Sprintf("%s NOT LIKE ?", col))
			bindings = append(bindings, opMap["not_like"])
		case hasKey(opMap, "in"):
			vals := toSlice(opMap["in"])
			if len(vals) == 0 {
				parts = append(parts, "1 = 0")
			} else {
				ph := strings.TrimSuffix(strings.Repeat("?, ", len(vals)), ", ")
				parts = append(parts, fmt.Sprintf("%s IN (%s)", col, ph))
				bindings = append(bindings, vals...)
			}
		case hasKey(opMap, "not_in"):
			vals := toSlice(opMap["not_in"])
			if len(vals) > 0 {
				ph := strings.TrimSuffix(strings.Repeat("?, ", len(vals)), ", ")
				parts = append(parts, fmt.Sprintf("%s NOT IN (%s)", col, ph))
				bindings = append(bindings, vals...)
			}
		default:
			parts = append(parts, fmt.Sprintf("%s = ?", col))
			bindings = append(bindings, op)
		}
	}

	return strings.Join(parts, " AND "), bindings
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`)+  `"`
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
