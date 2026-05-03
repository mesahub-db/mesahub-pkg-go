package mesahub

import (
	"fmt"
	"net/url"
)

// DatabaseHandle is a scoped entry point for a single database reference (UUID or slug).
//
// Obtained via Client.DB("ref").
type DatabaseHandle struct {
	ref    string
	client *Client
	Files  *DatabaseFiles
}

func newDatabaseHandle(ref string, c *Client) *DatabaseHandle {
	return &DatabaseHandle{
		ref:    ref,
		client: c,
		Files:  &DatabaseFiles{ref: ref, client: c},
	}
}

// Query executes a read-only SQL statement against this database.
func (d *DatabaseHandle) Query(sql string, bindings []any) (*QueryResult, error) {
	return d.client.Query(d.ref, sql, bindings)
}

// Exec executes a write SQL statement against this database.
func (d *DatabaseHandle) Exec(sql string, bindings []any) (*ExecResult, error) {
	return d.client.Exec(d.ref, sql, bindings)
}

// execRows is used internally by TableHandle for INSERT … RETURNING *.
func (d *DatabaseHandle) execRows(sql string, bindings []any) (*QueryResult, error) {
	raw, err := d.client.post(d.client.pathExec(d.ref), map[string]any{"sql": sql, "bindings": safe(bindings)})
	if err != nil {
		return nil, err
	}
	return parseQueryResult(raw), nil
}

// Table returns a TableHandle for the given table name.
func (d *DatabaseHandle) Table(tableName string) *TableHandle {
	return newTableHandle(tableName, d.Query, d.Exec, d.execRows)
}

// ── Files namespace ───────────────────────────────────────────────────────────

// DatabaseFiles provides file operations scoped to a database reference.
// Accessed via DatabaseHandle.Files.
type DatabaseFiles struct {
	ref    string
	client *Client
}

// List returns files stored in this database. Pass nil to omit optional params.
func (f *DatabaseFiles) List(limit, offset *int, folderPrefix *string) (map[string]any, error) {
	params := url.Values{}
	if limit != nil {
		params.Set("limit", fmt.Sprintf("%d", *limit))
	}
	if offset != nil {
		params.Set("offset", fmt.Sprintf("%d", *offset))
	}
	if folderPrefix != nil {
		params.Set("folder_prefix", *folderPrefix)
	}
	return f.client.get(f.client.pathFiles(f.ref), params)
}

// Upload uploads a file via multipart/form-data and returns the FileRecord.
func (f *DatabaseFiles) Upload(data []byte, filename, contentType string) (*FileRecord, error) {
	raw, err := f.client.upload(f.client.pathFiles(f.ref), data, filename, contentType)
	if err != nil {
		return nil, err
	}
	return fileRecordFromMap(raw), nil
}

// Download fetches the raw bytes for a file by ID.
func (f *DatabaseFiles) Download(fileID string) ([]byte, error) {
	return f.client.download(f.client.pathFileItem(f.ref, fileID))
}

// Delete removes a file by ID.
func (f *DatabaseFiles) Delete(fileID string) error {
	return f.client.delete(f.client.pathFileItem(f.ref, fileID))
}
