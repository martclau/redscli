package redscli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// writeBody renders an API response according to the selected output format.
func writeBody(out io.Writer, body []byte, format string) error {
	switch format {
	case "", "pretty":
		return writePretty(out, body)
	case "json":
		return writeCompactJSON(out, body)
	case "raw", "ndjson":
		_, err := out.Write(body)
		if err != nil {
			return err
		}
		return ensureTrailingNewline(out, body)
	default:
		return fmt.Errorf("unknown output format %q; use pretty, json, raw, or ndjson", format)
	}
}

// writePretty pretty-prints JSON but preserves non-JSON bodies unchanged.
func writePretty(out io.Writer, body []byte) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		_, writeErr := out.Write(body)
		if writeErr != nil {
			return writeErr
		}
		return ensureTrailingNewline(out, body)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

// writeCompactJSON emits minified JSON for scripts and downstream tools.
func writeCompactJSON(out io.Writer, body []byte) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		_, writeErr := out.Write(body)
		if writeErr != nil {
			return writeErr
		}
		return ensureTrailingNewline(out, body)
	}
	compact.WriteByte('\n')
	_, err := out.Write(compact.Bytes())
	return err
}

// marshalCompact serializes request or combined pagination payloads without
// HTML escaping so REDS query strings remain readable.
func marshalCompact(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// ensureTrailingNewline keeps terminal output clean for raw and non-JSON
// responses.
func ensureTrailingNewline(out io.Writer, body []byte) error {
	if len(body) == 0 || body[len(body)-1] == '\n' {
		return nil
	}
	_, err := io.WriteString(out, "\n")
	return err
}

// redactToken masks tokens when showing config data.
func redactToken(token string) string {
	token = strings.TrimSpace(token)
	switch {
	case token == "":
		return ""
	case len(token) <= 8:
		return "********"
	default:
		return token[:4] + "..." + token[len(token)-4:]
	}
}
