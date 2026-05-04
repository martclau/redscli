package redscli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

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
