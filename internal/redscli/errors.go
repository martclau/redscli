package redscli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

func withExitCode(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

type httpError struct {
	statusCode int
	status     string
	body       []byte
}

func (e *httpError) Error() string {
	msg := responseErrorMessage(e.body)
	if msg == "" {
		return e.status
	}
	return fmt.Sprintf("%s: %s", e.status, msg)
}

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
