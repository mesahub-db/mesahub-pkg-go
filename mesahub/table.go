package mesahub

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type (
	queryFnType  func(sql string, bindings []any) (*QueryResult, error)
	execFnType   func(sql string, bindings []any) (*ExecResult, error)
	wQueryFnType func(sql string, bindings []any) (*QueryResult, error)
)

// TableHandle provides high-level access to a single SQLite table.
//
// Obtained via DatabaseHandle.Table("table_name").
//
// All methods generate parameterised SQL — no string interpolation, injection-safe.
//
// Example:
//
//	users := client.DB("my-db").Table("users")
//
//	rows, _  := users.Find(FindOptions{Where: WhereClause{"active": 1}, Limit: intPtr(20)})
//	alice, _ := users.FindOne(WhereClause{"email": "alice@example.com"})
//	count, _ := users.Count(WhereClause{"active": 1})
//	row, _   := users.Insert(map[string]any{"name": "Bob", "email": "bob@example.com"})
//
//	users.Update(UpdateOptions{Where: WhereClause{"id": row["id"]}, Set: map[string]any{"name": "Robert"}})
//	users.Delete(WhereClause{"id": row["id"]})
//
//	// Decode rows into a struct
//	type User struct { ID int `json:"id"`; Name string `json:"name"` }
//	typed, _ := Scan[User](rows)
type TableHandle struct {
	tbl      string
	queryFn  queryFnType
	execFn   execFnType
	wQueryFn wQueryFnType
}

// FindOptions configures a Find query.
type FindOptions struct {
	Where   WhereClause
	Select  []string
	OrderBy []OrderByClause
	Limit   *int
	Offset  *int
}

// OrderByClause specifies sort column and direction ("asc" or "desc").
type OrderByClause struct {
	Column    string
	Direction string
}

// UpdateOptions configures an Update call.
type UpdateOptions struct {
	Where WhereClause
	Set   map[string]any
}

// InsertManyOptions configures conflict handling for InsertMany.
type InsertManyOptions struct {
	OnConflict string // "ignore" or "replace"
}

func newTableHandle(tableName string, qFn queryFnType, eFn execFnType, wFn wQueryFnType) *TableHandle {
	return &TableHandle{
		tbl:      quoteIdent(tableName),
		queryFn:  qFn,
		execFn:   eFn,
		wQueryFn: wFn,
	}
}

// Find returns rows matching the options.
func (t *TableHandle) Find(opts FindOptions) ([]map[string]any, error) {
	sql, bindings := t.buildSelect(opts)
	result, err := t.queryFn(sql, bindings)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// FindOne returns the first matching row, or nil if not found.
func (t *TableHandle) FindOne(where WhereClause) (map[string]any, error) {
	lim := 1
	rows, err := t.Find(FindOptions{Where: where, Limit: &lim})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

// Count returns the number of rows matching the optional filter.
func (t *TableHandle) Count(where WhereClause) (int64, error) {
	whereSql, bindings := BuildWhere(where)
	whereClause := ""
	if whereSql != "" {
		whereClause = " WHERE " + whereSql
	}
	sql := fmt.Sprintf(`SELECT COUNT(*) AS "_count" FROM %s%s`, t.tbl, whereClause)
	result, err := t.queryFn(sql, bindings)
	if err != nil || len(result.Rows) == 0 {
		return 0, err
	}
	switch v := result.Rows[0]["_count"].(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	}
	return 0, nil
}

// Insert inserts a single row and returns it (uses RETURNING *).
// Requires SQLite 3.35+ (all modern mesahub builds include this).
func (t *TableHandle) Insert(data map[string]any) (map[string]any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("mesahub: Insert called with empty data")
	}
	keys := sortedKeys(data)
	cols := strings.Join(quoteIdents(keys), ", ")
	phs := strings.Join(repeatN("?", len(keys)), ", ")
	bindings := make([]any, len(keys))
	for i, k := range keys {
		bindings[i] = data[k]
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *", t.tbl, cols, phs)
	result, err := t.wQueryFn(sql, bindings)
	if err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("mesahub: Insert returned no row from RETURNING *")
	}
	return result.Rows[0], nil
}

// InsertMany inserts multiple rows. Returns ExecResult (no per-row RETURNING).
func (t *TableHandle) InsertMany(rows []map[string]any, opts InsertManyOptions) (*ExecResult, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("mesahub: InsertMany called with empty rows")
	}
	keys := sortedKeys(rows[0])
	if len(keys) == 0 {
		return nil, fmt.Errorf("mesahub: InsertMany first row has no columns")
	}
	cols := strings.Join(quoteIdents(keys), ", ")
	rowPh := "(" + strings.Join(repeatN("?", len(keys)), ", ") + ")"
	conflict := ""
	switch opts.OnConflict {
	case "ignore":
		conflict = " OR IGNORE"
	case "replace":
		conflict = " OR REPLACE"
	}
	allPh := strings.Join(repeatN(rowPh, len(rows)), ", ")
	bindings := make([]any, 0, len(rows)*len(keys))
	for _, row := range rows {
		for _, k := range keys {
			bindings = append(bindings, row[k])
		}
	}
	sql := fmt.Sprintf("INSERT%s INTO %s (%s) VALUES %s", conflict, t.tbl, cols, allPh)
	return t.execFn(sql, bindings)
}

// Update updates rows matching opts.Where. A non-empty Where is required.
func (t *TableHandle) Update(opts UpdateOptions) (*ExecResult, error) {
	setKeys := sortedKeys(opts.Set)
	if len(setKeys) == 0 {
		return nil, fmt.Errorf("mesahub: Update called with empty set")
	}
	setClauses := make([]string, len(setKeys))
	setBindings := make([]any, len(setKeys))
	for i, k := range setKeys {
		setClauses[i] = quoteIdent(k) + " = ?"
		setBindings[i] = opts.Set[k]
	}
	whereSql, whereBindings := BuildWhere(opts.Where)
	if whereSql == "" {
		return nil, fmt.Errorf("mesahub: Update requires a non-empty where clause")
	}
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s", t.tbl, strings.Join(setClauses, ", "), whereSql)
	bindings := append(setBindings, whereBindings...)
	return t.execFn(sql, bindings)
}

// Delete deletes rows matching the where clause. A non-empty where is required.
func (t *TableHandle) Delete(where WhereClause) (*ExecResult, error) {
	whereSql, bindings := BuildWhere(where)
	if whereSql == "" {
		return nil, fmt.Errorf("mesahub: Delete requires a non-empty where clause")
	}
	sql := fmt.Sprintf("DELETE FROM %s WHERE %s", t.tbl, whereSql)
	return t.execFn(sql, bindings)
}

// Scan decodes a []map[string]any result into a typed slice via JSON round-trip.
//
//	type User struct { ID int `json:"id"`; Name string `json:"name"` }
//	users, err := mesahub.Scan[User](rows)
func Scan[T any](rows []map[string]any) ([]T, error) {
	result := make([]T, 0, len(rows))
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (t *TableHandle) buildSelect(opts FindOptions) (string, []any) {
	selectCols := "*"
	if len(opts.Select) > 0 {
		selectCols = strings.Join(quoteIdents(opts.Select), ", ")
	}

	whereSql, bindings := BuildWhere(opts.Where)
	whereClause := ""
	if whereSql != "" {
		whereClause = " WHERE " + whereSql
	}

	orderClause := ""
	if len(opts.OrderBy) > 0 {
		parts := make([]string, len(opts.OrderBy))
		for i, o := range opts.OrderBy {
			dir := strings.ToUpper(o.Direction)
			if dir == "" {
				dir = "ASC"
			}
			parts[i] = quoteIdent(o.Column) + " " + dir
		}
		orderClause = " ORDER BY " + strings.Join(parts, ", ")
	}

	limitClause := ""
	if opts.Limit != nil {
		limitClause = fmt.Sprintf(" LIMIT %d", *opts.Limit)
	}
	offsetClause := ""
	if opts.Offset != nil {
		offsetClause = fmt.Sprintf(" OFFSET %d", *opts.Offset)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s%s%s%s%s",
		selectCols, t.tbl, whereClause, orderClause, limitClause, offsetClause)
	return sql, bindings
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func quoteIdents(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = quoteIdent(k)
	}
	return out
}

func repeatN(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}
