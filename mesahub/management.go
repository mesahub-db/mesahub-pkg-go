package mesahub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

// ManagementConfig configures a ManagementClient.
type ManagementConfig struct {
	// DashboardURL is the MesaHub dashboard origin — scheme + host + optional port, no path.
	// e.g. "https://www.mesahub.app" or "http://localhost:3000"
	DashboardURL string
	// APIKey is the shs_ API key for the authenticated user.
	APIKey string
	// HTTPClient allows injecting a custom *http.Client (optional; defaults to http.DefaultClient).
	HTTPClient *http.Client
}

// DatabaseRecord represents a user database returned by the management API.
type DatabaseRecord struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	SizeBytes   int64   `json:"size_bytes"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// BucketRecord represents a user bucket returned by the management API.
type BucketRecord struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	SizeBytes   int64   `json:"size_bytes"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// APIKeyRecord represents an API key returned by the management API.
type APIKeyRecord struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	Status     string   `json:"status"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt *string  `json:"last_used_at"`
}

// CreateAPIKeyResult is returned when a new API key is created.
type CreateAPIKeyResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Key is the raw token — only returned on creation, never stored.
	Key string `json:"key"`
}

// ImportResult is returned after a successful Phase 2 import.
type ImportResult struct {
	RowsImported *int64 `json:"rows_imported"`
	SizeBytes    *int64 `json:"size_bytes"`
}

// ImportInspectResult is returned by Phase 1 of an import (no tables specified).
type ImportInspectResult struct {
	Tables []string `json:"tables"`
}

// ManagementClient is the MesaHub management-plane client.
// It talks to the MesaHub dashboard (Next.js) using a shs_ API key and provides
// CRUD for databases, buckets, and API keys.
//
// Usage:
//
//	mgmt := mesahub.NewManagementClient(mesahub.ManagementConfig{
//	    DashboardURL: "https://www.mesahub.app",
//	    APIKey:       "shs_...",
//	})
//	dbs, err := mgmt.ListDatabases()
type ManagementClient struct {
	http    *http.Client
	baseURL string
	auth    string
}

// NewManagementClient creates a new ManagementClient.
func NewManagementClient(cfg ManagementConfig) *ManagementClient {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	base := strings.TrimRight(cfg.DashboardURL, "/")
	return &ManagementClient{
		http:    hc,
		baseURL: base,
		auth:    "Bearer " + cfg.APIKey,
	}
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (m *ManagementClient) do(method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, m.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", m.auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return m.http.Do(req)
}

func (m *ManagementClient) decode(res *http.Response, out any) error {
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	if res.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (m *ManagementClient) req(method, path string, body any, out any) error {
	res, err := m.do(method, path, body)
	if err != nil {
		return err
	}
	return m.decode(res, out)
}

// ── Databases ─────────────────────────────────────────────────────────────────

// ListDatabases returns all databases owned by the authenticated user.
func (m *ManagementClient) ListDatabases() ([]DatabaseRecord, error) {
	var out []DatabaseRecord
	return out, m.req("GET", "/api/user/databases", nil, &out)
}

// GetDatabase returns a single database by ID.
func (m *ManagementClient) GetDatabase(id string) (*DatabaseRecord, error) {
	var out DatabaseRecord
	return &out, m.req("GET", "/api/user/databases/"+id, nil, &out)
}

// CreateDatabase creates a new database. name must be 3–50 lowercase alphanumeric
// characters, dashes, or underscores.
func (m *ManagementClient) CreateDatabase(name string, description string) (*DatabaseRecord, error) {
	body := map[string]any{"name": name}
	if description != "" {
		body["description"] = description
	}
	var out DatabaseRecord
	return &out, m.req("POST", "/api/user/databases", body, &out)
}

// DeleteDatabase deletes a database by ID.
func (m *ManagementClient) DeleteDatabase(id string) error {
	return m.req("DELETE", "/api/user/databases/"+id, nil, nil)
}

// UpdateDatabase updates a database's name and/or description.
func (m *ManagementClient) UpdateDatabase(id string, name string, description string) (*DatabaseRecord, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if description != "" {
		body["description"] = description
	}
	var out DatabaseRecord
	return &out, m.req("PATCH", "/api/user/databases/"+id, body, &out)
}

// ExportDatabase exports the given tables from a database as a SQLite file.
// The caller is responsible for closing the returned ReadCloser.
func (m *ManagementClient) ExportDatabase(id string, tables []string, filename string) (io.ReadCloser, error) {
	body := map[string]any{"tables": tables}
	if filename != "" {
		body["filename"] = filename
	}
	res, err := m.do("POST", "/api/user/databases/"+id+"/export", body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		defer res.Body.Close()
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Error != "" {
			return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return res.Body, nil
}

// ImportDatabaseInspect performs Phase 1 of an import: uploads the SQLite file
// and returns the list of available tables without modifying the target database.
func (m *ManagementClient) ImportDatabaseInspect(id string, fileReader io.Reader, filename string) (*ImportInspectResult, error) {
	rc, ct, err := m.buildMultipart(fileReader, filename, nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", m.baseURL+"/api/user/databases/"+id+"/import", rc)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", m.auth)
	req.Header.Set("Content-Type", ct)
	res, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	var out ImportInspectResult
	return &out, m.decode(res, &out)
}

// ImportDatabase performs Phase 2 of an import: uploads the SQLite file and copies
// the given tables into the target database.
func (m *ManagementClient) ImportDatabase(id string, fileReader io.Reader, filename string, tables []string) (*ImportResult, error) {
	rc, ct, err := m.buildMultipart(fileReader, filename, tables)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", m.baseURL+"/api/user/databases/"+id+"/import", rc)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", m.auth)
	req.Header.Set("Content-Type", ct)
	res, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	var out ImportResult
	return &out, m.decode(res, &out)
}

func (m *ManagementClient) buildMultipart(fileReader io.Reader, filename string, tables []string) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(fw, fileReader); err != nil {
		return nil, "", err
	}

	if tables != nil {
		tablesJSON, err := json.Marshal(tables)
		if err != nil {
			return nil, "", err
		}
		if err := mw.WriteField("tables", string(tablesJSON)); err != nil {
			return nil, "", err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

// ── Buckets ───────────────────────────────────────────────────────────────────

// ListBuckets returns all buckets owned by the authenticated user.
func (m *ManagementClient) ListBuckets() ([]BucketRecord, error) {
	var out []BucketRecord
	return out, m.req("GET", "/api/user/buckets", nil, &out)
}

// GetBucket returns a single bucket by ID.
func (m *ManagementClient) GetBucket(id string) (*BucketRecord, error) {
	var out BucketRecord
	return &out, m.req("GET", "/api/user/buckets/"+id, nil, &out)
}

// CreateBucket creates a new bucket.
func (m *ManagementClient) CreateBucket(name string, description string) (*BucketRecord, error) {
	body := map[string]any{"name": name}
	if description != "" {
		body["description"] = description
	}
	var out BucketRecord
	return &out, m.req("POST", "/api/user/buckets", body, &out)
}

// DeleteBucket deletes a bucket by ID.
func (m *ManagementClient) DeleteBucket(id string) error {
	return m.req("DELETE", "/api/user/buckets/"+id, nil, nil)
}

// UpdateBucket updates a bucket's name and/or description.
func (m *ManagementClient) UpdateBucket(id string, name string, description string) (*BucketRecord, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if description != "" {
		body["description"] = description
	}
	var out BucketRecord
	return &out, m.req("PATCH", "/api/user/buckets/"+id, body, &out)
}

// ── API Keys ──────────────────────────────────────────────────────────────────

// ListAPIKeys returns all API keys for the authenticated user.
func (m *ManagementClient) ListAPIKeys() ([]APIKeyRecord, error) {
	var out []APIKeyRecord
	return out, m.req("GET", "/api/user/api-keys", nil, &out)
}

// CreateAPIKey creates a new API key. The raw token in the result is only
// returned once and is not stored by the server.
// scopes defaults to ["all:w"] if nil.
func (m *ManagementClient) CreateAPIKey(name string, scopes []string) (*CreateAPIKeyResult, error) {
	if scopes == nil {
		scopes = []string{"all:w"}
	}
	var out CreateAPIKeyResult
	return &out, m.req("POST", "/api/user/api-keys", map[string]any{"name": name, "scopes": scopes}, &out)
}

// RevokeAPIKey revokes an API key by ID.
func (m *ManagementClient) RevokeAPIKey(id string) error {
	return m.req("DELETE", "/api/user/api-keys/"+id, nil, nil)
}
