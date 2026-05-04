package redscli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// exitError carries a stable CLI exit code with a regular Go error.
type exitError struct {
	code int
	err  error
}

// Error returns the user-facing message for an error with an explicit exit code.
func (e *exitError) Error() string {
	return e.err.Error()
}

// Unwrap exposes the underlying error for errors.As and errors.Is callers.
func (e *exitError) Unwrap() error {
	return e.err
}

// withExitCode creates a usage or runtime error that maps to a stable process
// exit code.
func withExitCode(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

// httpError represents a non-2xx REDS response.
type httpError struct {
	statusCode int
	status     string
	body       []byte
}

// Error combines the HTTP status with the best message available in the REDS
// error envelope.
func (e *httpError) Error() string {
	msg := responseErrorMessage(e.body)
	if msg == "" {
		return e.status
	}
	return fmt.Sprintf("%s: %s", e.status, msg)
}

// responseErrorMessage extracts a concise message from API error JSON, falling
// back to a trimmed text body.
func responseErrorMessage(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		text := strings.TrimSpace(string(body))
		if len(text) > 300 {
			return text[:300] + "..."
		}
		return text
	}
	return findMessage(value)
}

// findMessage recursively searches common REDS error-envelope fields.
func findMessage(value any) string {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"message", "detail", "error", "error_code", "code"} {
			if raw, ok := v[key]; ok {
				switch typed := raw.(type) {
				case string:
					if typed != "" {
						return typed
					}
				case map[string]any, []any:
					if nested := findMessage(typed); nested != "" {
						return nested
					}
				}
			}
		}
	case []any:
		if len(v) > 0 {
			return findMessage(v[0])
		}
	}
	return ""
}

// exitCode maps internal and HTTP errors to documented CLI exit codes.
func exitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	var he *httpError
	if errors.As(err, &he) {
		switch he.statusCode {
		case http.StatusUnauthorized:
			return 4
		case http.StatusForbidden:
			return 5
		case http.StatusNotFound:
			return 6
		case http.StatusTooManyRequests:
			return 7
		default:
			return 3
		}
	}
	return 1
}
