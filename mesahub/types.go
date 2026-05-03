// Package mesahub is the Go SDK for mesahub.
// It provides direct access to mesahub SQLite databases with zero external dependencies.
package mesahub

// QueryResult holds the results of a read-only SQL query.
type QueryResult struct {
	Rows            []map[string]any
	Columns         []string
	RowCount        int
	QueryDurationMs *float64
}

// ExecResult holds the results of a write SQL statement.
type ExecResult struct {
	RowsAffected    int64
	LastInsertRowid *int64
	QueryDurationMs *float64
}

// FileRecord represents a file stored alongside a database.
type FileRecord struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	FolderPath  *string `json:"folder_path"`
	SizeBytes   int64   `json:"size_bytes"`
	ContentType *string `json:"content_type"`
	URL         string  `json:"url"`
	UploadedAt  string  `json:"uploaded_at"`
	ExpiresAt   *string `json:"expires_at"`
	Metadata    *string `json:"metadata"`
}
