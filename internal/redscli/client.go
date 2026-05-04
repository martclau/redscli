package redscli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://reds.rationaledge.io/api/v1"
	userAgent      = "redscli/0.1.0"
)

type client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	verbose    bool
	errOut     io.Writer
}

type apiResponse struct {
	statusCode int
	header     http.Header
	body       []byte
}

func newClient(baseURL, token string, timeout time.Duration, verbose bool, errOut io.Writer) (*client, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q: expected absolute URL", baseURL)
	}
	if strings.TrimSpace(token) == "" {
		return nil, withExitCode(2, "missing API token; set REDS_API_TOKEN, pass --token, or run `redscli config set token <token>`")
	}
	return &client{
		baseURL: parsed,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		verbose: verbose,
		errOut:  errOut,
	}, nil
}

func (c *client) do(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, contentType string, headers http.Header) (*apiResponse, error) {
	req, err := c.newRequest(ctx, method, endpoint, query, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	result := &apiResponse{statusCode: resp.StatusCode, header: resp.Header.Clone(), body: data}
	if resp.StatusCode >= 400 {
		return result, &httpError{statusCode: resp.StatusCode, status: resp.Status, body: data}
	}
	return result, nil
}

func (c *client) doStream(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, contentType string, headers http.Header) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, endpoint, query, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, &httpError{statusCode: resp.StatusCode, status: resp.Status, body: data}
	}
	return resp, nil
}

func (c *client) newRequest(ctx context.Context, method, endpoint string, query url.Values, body io.Reader) (*http.Request, error) {
	u := *c.baseURL
	cleanBase := strings.TrimRight(u.Path, "/")
	cleanEndpoint := "/" + strings.TrimLeft(endpoint, "/")
	u.Path = path.Join(cleanBase, cleanEndpoint)
	if strings.HasSuffix(cleanEndpoint, "/") && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawQuery = query.Encode()

	if c.verbose && c.errOut != nil {
		fmt.Fprintf(c.errOut, "%s %s\n", method, u.String())
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json, application/x-ndjson;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

func jsonBody(v any) (io.Reader, string, error) {
	data, err := marshalCompact(v)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(data), "application/json", nil
}
