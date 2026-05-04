package redscli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

// String returns a comma-separated representation for flag package diagnostics.
func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

// Set appends one flag value.
func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// readValuesFile reads one bulk-search value per non-empty, non-comment line.
func readValuesFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var values []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		values = append(values, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// parseKeyValue parses command-line key=value pairs used by raw requests.
func parseKeyValue(value string) (string, string, error) {
	key, val, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", fmt.Errorf("expected key=value, got %q", value)
	}
	return strings.TrimSpace(key), val, nil
}
