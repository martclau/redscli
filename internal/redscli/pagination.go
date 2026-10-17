package redscli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// postAllBulkSearch re-encodes the POST body for every page, keeping the same
// field and values while advancing only the cursor.
func (a *app) postAllBulkSearch(ctx context.Context, payload bulkSearchRequest) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	var combined map[string]any
	items := make([]any, 0)
	seen := map[string]bool{payload.Cursor: true}
	pages := 0
	for {
		body, contentType, err := jsonBody(payload)
		if err != nil {
			return err
		}
		resp, err := c.do(ctx, http.MethodPost, "/bulk-search", nil, body, contentType, nil)
		if err != nil {
			return err
		}
		var page map[string]any
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return err
		}
		results, ok := page["results"].([]any)
		if !ok {
			return fmt.Errorf("bulk search pagination response is missing a results array")
		}
		if combined == nil {
			combined = page
		}
		items = append(items, results...)
		pages++
		hasMore, ok := page["has_more"].(bool)
		if !ok {
			return fmt.Errorf("bulk search pagination response is missing a has_more boolean")
		}
		if !hasMore {
			break
		}
		nextCursor, _ := page["next_cursor"].(string)
		if nextCursor == "" || seen[nextCursor] {
			return fmt.Errorf("bulk search pagination did not return a new cursor")
		}
		seen[nextCursor] = true
		payload.Cursor = nextCursor
	}
	combined["results"] = items
	combined["pages"] = pages
	combined["has_more"] = false
	combined["next_cursor"] = nil
	body, err := marshalCompact(combined)
	if err != nil {
		return err
	}
	return writeBody(a.out, body, a.opts.format)
}

// getAllJobs uses the jobs endpoint's top-level total/offset fields. Unlike
// string sample pivots, jobs does not return a nested pagination object.
func (a *app) getAllJobs(ctx context.Context, params url.Values, startOffset int) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	query := cloneValues(params)
	query.Set("offset", strconv.Itoa(startOffset))
	var combined map[string]any
	items := make([]any, 0)
	offset := startOffset
	pages := 0
	for {
		resp, err := c.do(ctx, http.MethodGet, "/jobs", query, nil, "", nil)
		if err != nil {
			return err
		}
		var page map[string]any
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return err
		}
		jobs, ok := page["jobs"].([]any)
		if !ok {
			return fmt.Errorf("jobs pagination response is missing a jobs array")
		}
		total := intFromAny(page["total"])
		if total < 0 || intFromAny(page["offset"]) != offset {
			return fmt.Errorf("jobs pagination response has an invalid total or offset")
		}
		if combined == nil {
			combined = page
		}
		items = append(items, jobs...)
		pages++
		nextOffset := offset + len(jobs)
		if len(jobs) == 0 || nextOffset >= total {
			break
		}
		if nextOffset <= offset {
			return fmt.Errorf("jobs pagination did not advance the offset")
		}
		offset = nextOffset
		query.Set("offset", strconv.Itoa(offset))
	}
	combined["jobs"] = items
	combined["pages"] = pages
	combined["offset"] = startOffset
	combined["limit"] = len(items)
	body, err := marshalCompact(combined)
	if err != nil {
		return err
	}
	return writeBody(a.out, body, a.opts.format)
}

// cursorPager describes REDS endpoints that expose top-level cursor pagination.
type cursorPager struct {
	itemsKey    string
	cursorParam string
}

// getAllCursor follows top-level has_more/next_cursor pagination and combines
// all item arrays into one response object.
func (a *app) getAllCursor(ctx context.Context, endpoint string, params url.Values, pager cursorPager) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	query := cloneValues(params)
	var combined map[string]any
	var items []any
	pages := 0

	for {
		resp, err := c.do(ctx, http.MethodGet, endpoint, query, nil, "", nil)
		if err != nil {
			return err
		}
		var page map[string]any
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return err
		}
		if combined == nil {
			combined = page
		}
		if raw, ok := page[pager.itemsKey].([]any); ok {
			items = append(items, raw...)
		}
		pages++

		hasMore, _ := page["has_more"].(bool)
		nextCursor, _ := page["next_cursor"].(string)
		if !hasMore || nextCursor == "" {
			break
		}
		query.Set(pager.cursorParam, nextCursor)
	}

	if combined == nil {
		combined = map[string]any{}
	}
	combined[pager.itemsKey] = items
	combined["pages"] = pages
	combined["has_more"] = false
	combined["next_cursor"] = nil
	body, err := marshalCompact(combined)
	if err != nil {
		return err
	}
	return writeBody(a.out, body, a.opts.format)
}

// nestedCursorPager describes REDS endpoints where pagination metadata is under
// a "pagination" object.
type nestedCursorPager struct {
	itemsKey    string
	cursorParam string
}

// getAllNestedCursor follows nested pagination and combines the endpoint's data
// array while preserving first-page metadata.
func (a *app) getAllNestedCursor(ctx context.Context, endpoint string, params url.Values, pager nestedCursorPager) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	query := cloneValues(params)
	var combined map[string]any
	var items []any
	pages := 0

	for {
		resp, err := c.do(ctx, http.MethodGet, endpoint, query, nil, "", nil)
		if err != nil {
			return err
		}
		var page map[string]any
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return err
		}
		if combined == nil {
			combined = page
		}
		if raw, ok := page[pager.itemsKey].([]any); ok {
			items = append(items, raw...)
		}
		pages++

		pagination, _ := page["pagination"].(map[string]any)
		hasMore, _ := pagination["has_more"].(bool)
		nextCursor := stringify(pagination["next_cursor"])
		if !hasMore || nextCursor == "" {
			break
		}
		query.Set(pager.cursorParam, nextCursor)
	}

	if combined == nil {
		combined = map[string]any{}
	}
	combined[pager.itemsKey] = items
	combined["pages"] = pages
	if pagination, ok := combined["pagination"].(map[string]any); ok {
		pagination["has_more"] = false
		pagination["next_cursor"] = nil
	}
	body, err := marshalCompact(combined)
	if err != nil {
		return err
	}
	return writeBody(a.out, body, a.opts.format)
}

// cloneValues copies query parameters before pagination mutates cursors.
func cloneValues(values url.Values) url.Values {
	if values == nil {
		return url.Values{}
	}
	out := make(url.Values, len(values))
	for key, list := range values {
		out[key] = append([]string(nil), list...)
	}
	return out
}

// getAllOffset follows offset-based pagination, used by string sample pivots.
func (a *app) getAllOffset(ctx context.Context, endpoint string, params url.Values, itemsKey string, offsetParam string, limitParam string) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	query := cloneValues(params)
	var combined map[string]any
	var items []any
	pages := 0

	for {
		resp, err := c.do(ctx, http.MethodGet, endpoint, query, nil, "", nil)
		if err != nil {
			return err
		}
		var page map[string]any
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return err
		}
		if combined == nil {
			combined = page
		}
		raw, _ := page[itemsKey].([]any)
		items = append(items, raw...)
		pages++

		pagination, _ := page["pagination"].(map[string]any)
		hasMore, _ := pagination["has_more"].(bool)
		if !hasMore {
			break
		}
		nextOffset := len(items)
		if currentOffset := intFromAny(pagination["offset"]); currentOffset >= 0 {
			nextOffset = currentOffset + len(raw)
		}
		if nextOffset <= 0 {
			break
		}
		query.Set(offsetParam, strconv.Itoa(nextOffset))
		if limit := stringify(pagination["limit"]); limit != "" && query.Get(limitParam) == "" {
			query.Set(limitParam, limit)
		}
	}

	if combined == nil {
		combined = map[string]any{}
	}
	combined[itemsKey] = items
	combined["pages"] = pages
	if pagination, ok := combined["pagination"].(map[string]any); ok {
		pagination["has_more"] = false
		pagination["offset"] = 0
		pagination["limit"] = len(items)
	}
	body, err := marshalCompact(combined)
	if err != nil {
		return err
	}
	return writeBody(a.out, body, a.opts.format)
}

// stringify converts JSON-decoded cursor values into query parameter text.
func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// intFromAny converts JSON-decoded numeric pagination fields into ints.
func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}
	return -1
}
