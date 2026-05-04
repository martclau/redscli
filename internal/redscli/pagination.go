package redscli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type cursorPager struct {
	itemsKey    string
	cursorParam string
}

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

type nestedCursorPager struct {
	itemsKey    string
	cursorParam string
}

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
