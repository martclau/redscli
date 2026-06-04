package redscli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestParseFlagsAllowsFlagsAfterPositionals verifies the CLI accepts common
// flag placement such as `upload sample.exe --private`.
func TestParseFlagsAllowsFlagsAfterPositionals(t *testing.T) {
	fs := newFlagSet("upload")
	private := fs.Bool("private", false, "")
	if err := parseFlags(fs, []string{"sample.exe", "--private"}); err != nil {
		t.Fatal(err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "sample.exe" {
		t.Fatalf("positionals = %v", fs.Args())
	}
	if !*private {
		t.Fatal("private flag was not set")
	}
}

// TestParseFlagsConsumesStringValue protects quoted REDS query values from
// being split or dropped during command parsing.
func TestParseFlagsConsumesStringValue(t *testing.T) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	query := fs.String("q", "", "")
	if err := parseFlags(fs, []string{"-q", `compiler:"Visual Basic"`}); err != nil {
		t.Fatal(err)
	}
	if *query != `compiler:"Visual Basic"` {
		t.Fatalf("query = %q", *query)
	}
}

// TestReadValuesFile verifies bulk-search files ignore blank lines and
// comments.
func TestReadValuesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.txt")
	if err := os.WriteFile(path, []byte("\n# comment\nabc\n def \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := readValuesFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"abc", "def"}
	if len(values) != len(want) {
		t.Fatalf("values = %#v", values)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("values = %#v", values)
		}
	}
}

// TestMainBulkSearch exercises end-to-end command parsing and JSON request
// creation for the bulk search command.
func TestMainBulkSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/bulk-search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload struct {
			Field  string   `json:"field"`
			Values []string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Field != "sha256" || len(payload.Values) != 2 || payload.Values[0] != "a" || payload.Values[1] != "b" {
			t.Fatalf("payload = %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{
		"--config", configPath,
		"--base-url", server.URL,
		"--token", "token",
		"--format", "json",
		"bulk", "search",
		"--field", "sha256",
		"--value", "a",
		"--value", "b",
	}, bytes.NewReader(nil), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if got := out.String(); got != "{\"ok\":true}\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestMainDownloadRawStdout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/download/abc/raw" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte{0x00, 0x01, 0xff})
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{
		"--config", configPath,
		"--base-url", server.URL,
		"--token", "token",
		"download", "abc",
		"--raw",
		"--stdout",
	}, bytes.NewReader(nil), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if got, want := out.Bytes(), []byte{0x00, 0x01, 0xff}; !bytes.Equal(got, want) {
		t.Fatalf("stdout = %#v", got)
	}
}

func TestMainDownloadRawOutputFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/abc/raw" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("raw-bytes"))
	}))
	defer server.Close()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "sample.bin")
	configPath := filepath.Join(dir, "config.json")
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{
		"--config", configPath,
		"--base-url", server.URL,
		"--token", "token",
		"download", "abc",
		"--raw",
		"--output", outputPath,
	}, bytes.NewReader(nil), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := data, []byte("raw-bytes"); !bytes.Equal(got, want) {
		t.Fatalf("file = %#v", got)
	}
	if got := out.String(); got != "saved "+outputPath+" (9 bytes)\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestMainDownloadZipFallbackFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/abc" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer server.Close()

	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	})
	configPath := filepath.Join(dir, "config.json")
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{
		"--config", configPath,
		"--base-url", server.URL,
		"--token", "token",
		"download", "abc",
	}, bytes.NewReader(nil), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "abc.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := data, []byte("zip-bytes"); !bytes.Equal(got, want) {
		t.Fatalf("file = %#v", got)
	}
	if got := out.String(); got != "saved abc.zip (9 bytes)\nzip password: infected\n" {
		t.Fatalf("stdout = %q", got)
	}
}
