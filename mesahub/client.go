package mesahub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Config configures a Client.
type Config struct {
	// APIKey is the shs_ API key (or admin token).
	APIKey string
	// APIURL is the core server origin — scheme + host + optional port, no path.
	// e.g. "https://api.yourapp.com" or "http://localhost:4004"
	APIURL string
	// RoutePrefix selects the API path style: "v1" (default) or "api".
	RoutePrefix string
	// HTTPClient allows injecting a custom *http.Client (e.g. with custom TLS or timeouts).
	// Defaults to http.DefaultClient when nil.
	HTTPClient *http.Client
}

// Client is the mesahub data-plane client.
type Client struct {
	http         *http.Client
	baseURL      string
	authHeader   string
	pathQuery    func(ref string) string
	pathExec     func(ref string) string
	pathFiles    func(ref string) string
	pathFileItem func(ref, id string) string
}

// New creates a new Client from the given Config.
func New(cfg Config) *Client {
	stripSuffix := regexp.MustCompile(`/(v1|api)/?$`)
	base := stripSuffix.ReplaceAllString(strings.TrimRight(cfg.APIURL, "/"), "")

	prefix := cfg.RoutePrefix
	if prefix == "" {
		prefix = "v1"
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}

	c := &Client{
		http:       hc,
		baseURL:    base,
		authHeader: "Bearer " + cfg.APIKey,
	}

	if prefix == "api" {
		c.pathQuery = func(ref string) string { return "/api/db/" + ref + "/query" }
		c.pathExec = func(ref string) string { return "/api/db/" + ref + "/exec" }
		c.pathFiles = func(ref string) string { return "/api/db/" + ref + "/files" }
		c.pathFileItem = func(ref, id string) string { return "/api/db/" + ref + "/files/" + id }
	} else {
		c.pathQuery = func(ref string) string { return "/v1/query/" + ref }
		c.pathExec = func(ref string) string { return "/v1/exec/" + ref }
		c.pathFiles = func(ref string) string { return "/v1/files/" + ref }
		c.pathFileItem = func(ref, id string) string { return "/v1/files/" + ref + "/" + id }
	}

	return c
}

// Query executes a read-only SQL statement.
func (c *Client) Query(ref, sql string, bindings []any) (*QueryResult, error) {
	raw, err := c.post(c.pathQuery(ref), map[string]any{"sql": sql, "bindings": safe(bindings)})
	if err != nil {
		return nil, err
	}
	return parseQueryResult(raw), nil
}

// Exec executes a write SQL statement.
func (c *Client) Exec(ref, sql string, bindings []any) (*ExecResult, error) {
	raw, err := c.post(c.pathExec(ref), map[string]any{"sql": sql, "bindings": safe(bindings)})
	if err != nil {
		return nil, err
	}
	return parseExecResult(raw), nil
}

// DB returns a DatabaseHandle scoped to the given database reference (UUID or slug).
func (c *Client) DB(ref string) *DatabaseHandle {
	return newDatabaseHandle(ref, c)
}

// ── HTTP primitives ───────────────────────────────────────────────────────────

func (c *Client) post(path string, body any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("mesahub: marshal request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	return c.do(req)
}

func (c *Client) get(path string, params url.Values) (map[string]any, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	return c.do(req)
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader)
	_, err = c.do(req)
	return err
}

func (c *Client) upload(path string, data []byte, filename, contentType string) (map[string]any, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err = fw.Write(data); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", c.authHeader)
	return c.do(req)
}

func (c *Client) download(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mesahub: http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var errBody map[string]any
		_ = json.Unmarshal(body, &errBody)
		return nil, errorFromResponse(resp.StatusCode, resp.Status, errBody)
	}
	return body, nil
}

func (c *Client) do(req *http.Request) (map[string]any, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mesahub: http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("mesahub: decode response: %w", err)
		}
	}
	if resp.StatusCode >= 400 {
		return nil, errorFromResponse(resp.StatusCode, resp.Status, data)
	}
	return data, nil
}

// ── Response parsers ──────────────────────────────────────────────────────────

func parseQueryResult(raw map[string]any) *QueryResult {
	var columns []string
	if headers, ok := raw["headers"].([]any); ok {
		for _, h := range headers {
			if m, ok := h.(map[string]any); ok {
				if n, ok := m["name"].(string); ok {
					columns = append(columns, n)
				}
			}
		}
	} else if cols, ok := raw["columns"].([]any); ok {
		for _, col := range cols {
			if s, ok := col.(string); ok {
				columns = append(columns, s)
			}
		}
	}

	var rows []map[string]any
	if rawRows, ok := raw["rows"].([]any); ok {
		for _, r := range rawRows {
			if m, ok := r.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
	}

	var durMs *float64
	if stat, ok := raw["stat"].(map[string]any); ok {
		if d, ok := stat["queryDurationMs"].(float64); ok {
			durMs = &d
		}
	}

	return &QueryResult{
		Rows:            rows,
		Columns:         columns,
		RowCount:        len(rows),
		QueryDurationMs: durMs,
	}
}

func parseExecResult(raw map[string]any) *ExecResult {
	stat, _ := raw["stat"].(map[string]any)

	var rowsAffected int64
	if v, ok := raw["rowsAffected"].(float64); ok {
		rowsAffected = int64(v)
	} else if v, ok := stat["rowsAffected"].(float64); ok {
		rowsAffected = int64(v)
	}

	var lastID *int64
	if v, ok := raw["lastInsertRowid"].(float64); ok {
		id := int64(v)
		lastID = &id
	}

	var durMs *float64
	if d, ok := stat["queryDurationMs"].(float64); ok {
		durMs = &d
	}

	return &ExecResult{
		RowsAffected:    rowsAffected,
		LastInsertRowid: lastID,
		QueryDurationMs: durMs,
	}
}

func fileRecordFromMap(m map[string]any) *FileRecord {
	fr := &FileRecord{}
	if v, ok := m["id"].(string); ok {
		fr.ID = v
	}
	if v, ok := m["filename"].(string); ok {
		fr.Filename = v
	}
	if v, ok := m["folder_path"].(string); ok {
		fr.FolderPath = &v
	}
	if v, ok := m["size_bytes"].(float64); ok {
		fr.SizeBytes = int64(v)
	}
	if v, ok := m["content_type"].(string); ok {
		fr.ContentType = &v
	}
	if v, ok := m["url"].(string); ok {
		fr.URL = v
	}
	if v, ok := m["uploaded_at"].(string); ok {
		fr.UploadedAt = v
	}
	if v, ok := m["expires_at"].(string); ok {
		fr.ExpiresAt = &v
	}
	if v, ok := m["metadata"].(string); ok {
		fr.Metadata = &v
	}
	return fr
}

// safe returns an empty []any instead of nil so JSON serialises as [] not null.
func safe(s []any) []any {
	if s == nil {
		return []any{}
	}
	return s
}
