package redscli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

type paginationResponse struct {
	status int
	body   string
}

func runPaginationCommand(t *testing.T, baseURL string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	global := []string{"--config", filepath.Join(t.TempDir(), "config.json"),
		"--base-url", baseURL, "--token", "test-token", "--format", "json"}
	code := Main(append(global, args...), bytes.NewReader(nil), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestBulkSearchAll(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cursors []string
		pages   []paginationResponse
		code    int
		results int
	}{
		{
			name:    "multiple pages from starting cursor",
			cursors: []string{"start", "next"},
			pages: []paginationResponse{
				{200, `{"results":[{"sha256":"a"}],"total_count":2,"has_more":true,"next_cursor":"next","bulk_search_meta":{"unmatched_values":["2.2.2.2"]}}`},
				{200, `{"results":[{"sha256":"b"}],"has_more":false}`},
			},
			results: 2,
		},
		{
			name: "empty", cursors: []string{""},
			pages: []paginationResponse{{200, `{"results":[],"has_more":false}`}},
		},
		{
			name: "missing cursor", cursors: []string{""},
			pages: []paginationResponse{{200, `{"results":[],"has_more":true}`}},
			code:  1,
		},
		{
			name: "repeated cursor", cursors: []string{"", "next"},
			pages: []paginationResponse{
				{200, `{"results":[],"has_more":true,"next_cursor":"next"}`},
				{200, `{"results":[],"has_more":true,"next_cursor":"next"}`},
			},
			code: 1,
		},
		{
			name: "later API error", cursors: []string{"", "next"},
			pages: []paginationResponse{
				{200, `{"results":[{"sha256":"a"}],"has_more":true,"next_cursor":"next"}`},
				{429, `{"error":"quota exceeded"}`},
			},
			code: 7,
		},
		{
			name: "malformed response", cursors: []string{""},
			pages: []paginationResponse{{200, `not JSON`}}, code: 1,
		},
		{
			name: "missing results", cursors: []string{""},
			pages: []paginationResponse{{200, `{"has_more":false}`}}, code: 1,
		},
		{
			name: "missing pagination flag", cursors: []string{""},
			pages: []paginationResponse{{200, `{"results":[]}`}}, code: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				index := calls
				calls++
				if index >= len(tc.pages) {
					t.Error("unexpected extra page request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/bulk-search" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q", got)
				}
				var payload bulkSearchRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Field != "ipv4" || !reflect.DeepEqual(payload.Values, []string{"1.1.1.1", "2.2.2.2"}) || payload.Cursor != tc.cursors[index] {
					t.Errorf("page %d payload = %#v", index, payload)
				}
				w.WriteHeader(tc.pages[index].status)
				_, _ = w.Write([]byte(tc.pages[index].body))
			}))
			defer server.Close()
			args := []string{"bulk", "search", "--field", "ipv4", "--values", "1.1.1.1,2.2.2.2", "--all"}
			if tc.cursors[0] != "" {
				args = append(args, "--cursor", tc.cursors[0])
			}
			code, out, errOut := runPaginationCommand(t, server.URL, args...)
			if code != tc.code || calls != len(tc.pages) {
				t.Fatalf("exit = %d, calls = %d, stderr = %s", code, calls, errOut)
			}
			if code != 0 {
				if out != "" {
					t.Fatalf("partial stdout = %s", out)
				}
				return
			}
			var response map[string]any
			if err := json.Unmarshal([]byte(out), &response); err != nil {
				t.Fatal(err)
			}
			results, ok := response["results"].([]any)
			if !ok || len(results) != tc.results || response["pages"] != float64(len(tc.pages)) || response["has_more"] != false || response["next_cursor"] != nil {
				t.Fatalf("combined response = %s", out)
			}
			if tc.results == 2 {
				if results[0].(map[string]any)["sha256"] != "a" || results[1].(map[string]any)["sha256"] != "b" || response["total_count"] != float64(2) {
					t.Fatalf("combined results = %s", out)
				}
				meta := response["bulk_search_meta"].(map[string]any)
				if !reflect.DeepEqual(meta["unmatched_values"], []any{"2.2.2.2"}) {
					t.Fatalf("bulk metadata = %#v", meta)
				}
			}
		})
	}
}

func TestJobsAll(t *testing.T) {
	for _, tc := range []struct {
		name    string
		offsets []int
		pages   []paginationResponse
		code    int
		jobs    int
	}{
		{
			name: "multiple pages from starting offset", offsets: []int{3, 5}, jobs: 3,
			pages: []paginationResponse{
				{200, `{"jobs":[{"job_id":"a"},{"job_id":"b"}],"total":6,"limit":2,"offset":3}`},
				{200, `{"jobs":[{"job_id":"c"}],"total":6,"limit":2,"offset":5}`},
			},
		},
		{
			name: "exact page boundary", offsets: []int{0, 2}, jobs: 4,
			pages: []paginationResponse{
				{200, `{"jobs":[{"job_id":"a"},{"job_id":"b"}],"total":4,"limit":2,"offset":0}`},
				{200, `{"jobs":[{"job_id":"c"},{"job_id":"d"}],"total":4,"limit":2,"offset":2}`},
			},
		},
		{
			name: "empty", offsets: []int{0},
			pages: []paginationResponse{{200, `{"jobs":[],"total":0,"limit":2,"offset":0}`}},
		},
		{
			name: "empty past total", offsets: []int{10},
			pages: []paginationResponse{{200, `{"jobs":[],"total":6,"limit":2,"offset":10}`}},
		},
		{
			name: "empty page stops despite total", offsets: []int{0},
			pages: []paginationResponse{{200, `{"jobs":[],"total":6,"limit":2,"offset":0}`}},
		},
		{
			name: "later API error", offsets: []int{0, 1}, code: 7,
			pages: []paginationResponse{
				{200, `{"jobs":[{"job_id":"a"}],"total":2,"limit":2,"offset":0}`},
				{429, `{"error":"quota exceeded"}`},
			},
		},
		{
			name: "offset did not advance", offsets: []int{0, 1}, code: 1,
			pages: []paginationResponse{
				{200, `{"jobs":[{"job_id":"a"}],"total":2,"limit":2,"offset":0}`},
				{200, `{"jobs":[{"job_id":"a"}],"total":2,"limit":2,"offset":0}`},
			},
		},
		{
			name: "missing jobs", offsets: []int{0}, code: 1,
			pages: []paginationResponse{{200, `{"total":0,"offset":0}`}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				index := calls
				calls++
				if index >= len(tc.pages) {
					t.Error("unexpected extra page request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/jobs" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				query := r.URL.Query()
				if query.Get("status") != "completed" || query.Get("limit") != "2" || query.Get("offset") != strconv.Itoa(tc.offsets[index]) {
					t.Errorf("page %d query = %v", index, query)
				}
				w.WriteHeader(tc.pages[index].status)
				_, _ = w.Write([]byte(tc.pages[index].body))
			}))
			defer server.Close()
			code, out, errOut := runPaginationCommand(t, server.URL, "jobs", "--status", "completed", "--limit", "2", "--offset", strconv.Itoa(tc.offsets[0]), "--all")
			if code != tc.code || calls != len(tc.pages) {
				t.Fatalf("exit = %d, calls = %d, stderr = %s", code, calls, errOut)
			}
			if code != 0 {
				if out != "" {
					t.Fatalf("partial stdout = %s", out)
				}
				return
			}
			var response map[string]any
			if err := json.Unmarshal([]byte(out), &response); err != nil {
				t.Fatal(err)
			}
			jobs, ok := response["jobs"].([]any)
			if !ok || len(jobs) != tc.jobs || response["pages"] != float64(len(tc.pages)) || response["offset"] != float64(tc.offsets[0]) || response["limit"] != float64(tc.jobs) {
				t.Fatalf("combined response = %s", out)
			}
			for i, job := range jobs {
				if job.(map[string]any)["job_id"] != string(rune('a'+i)) {
					t.Fatalf("job order = %s", out)
				}
			}
		})
	}
}

func TestAllPaginationInputValidation(t *testing.T) {
	for _, args := range [][]string{
		{"jobs", "--all", "--limit", "0"},
		{"jobs", "--all", "--limit", "-1"},
		{"jobs", "--all", "--offset", "-1"},
	} {
		code, _, errOut := runPaginationCommand(t, "http://127.0.0.1:1", args...)
		if code != 2 {
			t.Fatalf("args = %v, exit = %d, stderr = %s", args, code, errOut)
		}
	}
	args := []string{"bulk", "search", "--field", "sha256", "--all"}
	for i := 0; i < 301; i++ {
		args = append(args, "--value", strconv.Itoa(i))
	}
	code, _, errOut := runPaginationCommand(t, "http://127.0.0.1:1", args...)
	if code != 2 {
		t.Fatalf("301-value bulk search exit = %d, stderr = %s", code, errOut)
	}
}
