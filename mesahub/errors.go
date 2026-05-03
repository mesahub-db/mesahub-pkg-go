package mesahub

import "fmt"

// MesahubError is returned when the server responds with an HTTP error status.
type MesahubError struct {
	Code       string
	StatusCode int
	Message    string
	Details    map[string]any
}

func (e *MesahubError) Error() string {
	return fmt.Sprintf("mesahub [%s] %d: %s", e.Code, e.StatusCode, e.Message)
}

func errorFromResponse(statusCode int, statusText string, body map[string]any) *MesahubError {
	code := "UNKNOWN_ERROR"
	message := statusText
	if v, ok := body["code"].(string); ok {
		code = v
	}
	if v, ok := body["message"].(string); ok {
		message = v
	}
	return &MesahubError{Code: code, StatusCode: statusCode, Message: message, Details: body}
}
