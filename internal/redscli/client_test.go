package redscli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientAddsAuthAndBuildsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != `compiler:"Visual Basic"` {
			t.Fatalf("q = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	c, err := newClient(server.URL+"/api/v1", "test-token", time.Second, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.do(context.Background(), http.MethodGet, "/search", url.Values{
		"q": []string{`compiler:"Visual Basic"`},
	}, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.statusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.statusCode)
	}
}

func TestClientHTTPErrorIncludesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   true,
			"code":    "bad_request",
			"message": "invalid query syntax",
		})
	}))
	defer server.Close()

	c, err := newClient(server.URL, "test-token", time.Second, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.do(context.Background(), http.MethodGet, "/search", nil, nil, "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "400 Bad Request: invalid query syntax" {
		t.Fatalf("error = %q", got)
	}
}
