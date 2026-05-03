# MesaHub Go SDK

Go SDK for [MesaHub](https://mesahub.app) — access SQLite databases from Go with raw SQL or a high-level table API.

Zero external dependencies — uses only the Go standard library.

## Installation

```bash
go get github.com/mesahub-db/mesahub-pkg-go
```

Requires Go 1.21+.

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/mesahub-db/mesahub-pkg-go/mesahub"
)

func main() {
    client := mesahub.New(mesahub.Config{
        APIKey: "shs_your_api_key",        // from mesahub.app → Settings → API Keys
        APIURL: "https://api.mesahub.app", // or your self-hosted core URL
    })

    db    := client.DB("my-app-db") // your database slug from the dashboard
    users := db.Table("users")

    // High-level table API
    rows, _  := users.Find(mesahub.FindOptions{
        Where: mesahub.WhereClause{"active": 1},
        Limit: intPtr(20),
    })

    alice, _ := users.FindOne(mesahub.WhereClause{"email": "alice@example.com"})
    count, _ := users.Count(mesahub.WhereClause{"active": 1})

    newRow, _ := users.Insert(map[string]any{
        "name":  "Bob",
        "email": "bob@example.com",
        "active": 1,
    })

    users.Update(mesahub.UpdateOptions{
        Where: mesahub.WhereClause{"id": newRow["id"]},
        Set:   map[string]any{"name": "Robert"},
    })
    users.Delete(mesahub.WhereClause{"id": newRow["id"]})

    // Raw SQL
    result, _ := db.Query("SELECT * FROM users WHERE active = ?", []any{1})
    fmt.Println(result.Rows)

    db.Exec("CREATE TABLE IF NOT EXISTS logs (msg TEXT, created_at TEXT)", nil)

    _ = rows; _ = alice; _ = count
}

func intPtr(n int) *int { return &n }
```

## Connection String

```go
info, err := mesahub.ParseURL("mh://shs_abc@mycore.railway.app/mydb")
if err != nil {
    log.Fatal(err)
}

client := mesahub.New(mesahub.Config{
    APIKey:      info.APIKey,
    APIURL:      info.APIURL,
    RoutePrefix: info.RoutePrefix,
})
db := client.DB(info.DBName)
```

Format: `mh://apikey@host[:port]/dbname`

- Remote hosts (e.g. `railway.app`) → HTTPS, `/v1/` routes
- `localhost` / Docker service names / `*.internal` → HTTP, `/api/` routes

## WHERE Filters

```go
// Shorthand equality
users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{"active": 1}})

// Comparison operators
users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{
    "age":   map[string]any{"gte": 18},
    "score": map[string]any{"lt": 100},
}})

// LIKE
users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{
    "name": map[string]any{"like": "%alice%"},
}})

// IN / NOT IN
users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{
    "role": map[string]any{"in": []any{"admin", "editor"}},
}})

// NULL checks
users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{
    "deleted_at": map[string]any{"is_null": true},
}})
```

All conditions in a single `WhereClause` are combined with `AND`.

| Operator key  | SQL equivalent   |
|---------------|------------------|
| *(plain value)* | `= ?`          |
| `eq`          | `= ?`           |
| `ne`          | `!= ?`          |
| `gt`          | `> ?`           |
| `gte`         | `>= ?`          |
| `lt`          | `< ?`           |
| `lte`         | `<= ?`          |
| `like`        | `LIKE ?`        |
| `not_like`    | `NOT LIKE ?`    |
| `in`          | `IN (...)`      |
| `not_in`      | `NOT IN (...)`  |
| `is_null`     | `IS NULL`       |
| `is_not_null` | `IS NOT NULL`   |

## `Find()` Options

```go
rows, err := users.Find(mesahub.FindOptions{
    Where:  mesahub.WhereClause{"active": 1},
    Select: []string{"id", "name", "email"},
    OrderBy: []mesahub.OrderByClause{
        {Column: "created_at", Direction: "desc"},
    },
    Limit:  intPtr(10),
    Offset: intPtr(20),
})
```

## `InsertMany()`

```go
db.Table("logs").InsertMany(
    []map[string]any{{"msg": "started"}, {"msg": "done"}},
    mesahub.InsertManyOptions{OnConflict: "ignore"}, // "ignore" | "replace" | ""
)
```

## Scan Into a Struct

Use `mesahub.Scan[T]` to decode `[]map[string]any` rows into a typed slice via JSON:

```go
type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

rows, _ := users.Find(mesahub.FindOptions{Where: mesahub.WhereClause{"active": 1}})
typed, err := mesahub.Scan[User](rows)
// typed is []User
```

## Files

```go
files := db.Files

// List
listing, _ := files.List(nil, nil, nil) // limit, offset, folderPrefix

// Upload
record, _ := files.Upload(pdfBytes, "report.pdf", "application/pdf")
fmt.Println(record.URL)

// Download
data, _ := files.Download(record.ID)

// Delete
files.Delete(record.ID)
```

## Error Handling

```go
rows, err := db.Query("SELECT * FROM users", nil)
if err != nil {
    if me, ok := err.(*mesahub.MesahubError); ok {
        fmt.Printf("[%s] %d: %s\n", me.Code, me.StatusCode, me.Message)
    }
    log.Fatal(err)
}
```

## Custom HTTP Client

```go
import "net/http"

client := mesahub.New(mesahub.Config{
    APIKey: "shs_...",
    APIURL: "https://api.mesahub.app",
    HTTPClient: &http.Client{
        Timeout: 10 * time.Second,
    },
})
```

## Self-Hosting

```go
client := mesahub.New(mesahub.Config{
    APIKey:      "your-admin-token",
    APIURL:      "http://localhost:4004",
    RoutePrefix: "api",
})
```
