package mesahub

import (
	"fmt"
	"net/url"
	"strings"
)

// ParsedURL holds the components from a parsed mh:// connection string.
type ParsedURL struct {
	APIURL      string
	APIKey      string
	DBName      string
	RoutePrefix string
}

// ParseURL parses a mh:// connection string into its component parts.
//
// Format: mh://apikey@host[:port]/dbname
//
// Examples:
//
//	ParseURL("mh://shs_abc@mycore.railway.app/mydb")   // → HTTPS, v1 prefix
//	ParseURL("mh://secret@localhost:4004/mydb")         // → HTTP,  api prefix
func ParseURL(raw string) (*ParsedURL, error) {
	if !strings.HasPrefix(raw, "mh://") {
		return nil, fmt.Errorf("mesahub: invalid URL: must start with mh:// (got: %q)", truncate(raw, 30))
	}

	// Replace scheme so stdlib url.Parse can handle it.
	parsed, err := url.Parse(strings.Replace(raw, "mh://", "http://", 1))
	if err != nil {
		return nil, fmt.Errorf("mesahub: invalid URL: %w", err)
	}

	host := parsed.Hostname()

	if host == "local" {
		return nil, fmt.Errorf(
			"mesahub: mh://local/... is the embedded mode placeholder — " +
				"it must be resolved to a concrete URL by start.sh before the application starts",
		)
	}

	isPrivate := host == "localhost" ||
		host == "127.0.0.1" ||
		!strings.Contains(host, ".") ||
		strings.HasSuffix(host, ".internal")

	scheme := "https"
	if isPrivate {
		scheme = "http"
	}
	portPart := ""
	if p := parsed.Port(); p != "" {
		portPart = ":" + p
	}
	apiURL := fmt.Sprintf("%s://%s%s", scheme, host, portPart)

	apiKey, err := url.QueryUnescape(parsed.User.Username())
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("mesahub: MESAHUB_URL must include an API key: mh://apikey@host/dbname")
	}

	dbName := strings.Trim(parsed.Path, "/")
	if dbName == "" {
		return nil, fmt.Errorf("mesahub: MESAHUB_URL must include a database name: mh://apikey@host/dbname")
	}

	routePrefix := "v1"
	if isPrivate {
		routePrefix = "api"
	}

	return &ParsedURL{
		APIURL:      apiURL,
		APIKey:      apiKey,
		DBName:      dbName,
		RoutePrefix: routePrefix,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
