package redscli

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// runStrings dispatches string search and per-sample string commands.
func (a *app) runStrings(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("strings")
		return nil
	}
	switch args[0] {
	case "search":
		return a.runStringsSearch(args[1:])
	case "samples":
		return a.runStringSamples(args[1:])
	case "stats", "statistics":
		return a.runSampleStringsGET("strings stats", "/samples/%s/strings/statistics", args[1:])
	case "list":
		return a.runSampleStringsList(args[1:])
	default:
		return withExitCode(2, "unknown strings subcommand %q", args[0])
	}
}

// runStringsSearch searches string-domain fields across the dataset.
func (a *app) runStringsSearch(args []string) error {
	fs := newFlagSet("strings search")
	query := fs.String("q", "", "string-domain query")
	fs.StringVar(query, "query", "", "string-domain query")
	cursor := fs.String("cursor", "", "pagination cursor")
	all := fs.Bool("all", false, "follow cursor pagination and combine results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *query == "" && fs.NArg() == 1 {
		*query = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return withExitCode(2, "strings search accepts at most one positional query")
	}
	if strings.TrimSpace(*query) == "" {
		return withExitCode(2, "strings search requires -q <query>")
	}
	params := url.Values{"q": []string{*query}}
	addQuery(params, "cursor", *cursor)
	if *all {
		return a.getAllCursor(context.Background(), "/strings/search", params, cursorPager{itemsKey: "results", cursorParam: "cursor"})
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/strings/search", params, nil, "", nil)
}

// runStringSamples retrieves samples containing an exact raw string value.
func (a *app) runStringSamples(args []string) error {
	fs := newFlagSet("strings samples")
	value := fs.String("string", "", "exact string value")
	offset := fs.Int("offset", 0, "pagination offset")
	limit := fs.Int("limit", 0, "number of samples, 1-100")
	all := fs.Bool("all", false, "follow offset pagination and combine data")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *value == "" && fs.NArg() == 1 {
		*value = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return withExitCode(2, "strings samples accepts at most one positional string value")
	}
	if *value == "" {
		return withExitCode(2, "strings samples requires --string <value>")
	}
	params := url.Values{"string": []string{*value}}
	if *offset != 0 {
		params.Set("offset", strconv.Itoa(*offset))
	}
	if *limit != 0 {
		params.Set("limit", strconv.Itoa(*limit))
	}
	if *all {
		return a.getAllOffset(context.Background(), "/strings/samples", params, "data", "offset", "limit")
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/strings/samples", params, nil, "", nil)
}

// runSampleStringsGET handles simple per-sample string endpoints.
func (a *app) runSampleStringsGET(command string, pattern string, args []string) error {
	fs := newFlagSet(command)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, command, "sha256")
	if err != nil {
		return err
	}
	endpoint := strings.Replace(pattern, "%s", url.PathEscape(sha256), 1)
	return a.requestJSON(context.Background(), http.MethodGet, endpoint, nil, nil, "", nil)
}

// runSampleStringsList lists strings extracted from one sample with optional
// filters and cursor pagination.
func (a *app) runSampleStringsList(args []string) error {
	fs := newFlagSet("strings list")
	cursor := fs.String("cursor", "", "pagination cursor")
	limit := fs.Int("limit", 0, "number of strings, 1-1000")
	search := fs.String("search", "", "search term")
	encoding := fs.String("encoding", "", "encoding filter")
	minEntropy := fs.String("min-entropy", "", "minimum entropy")
	maxEntropy := fs.String("max-entropy", "", "maximum entropy")
	all := fs.Bool("all", false, "follow cursor pagination and combine data")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, "strings list", "sha256")
	if err != nil {
		return err
	}
	params := url.Values{}
	addQuery(params, "cursor", *cursor)
	addQuery(params, "search", *search)
	addQuery(params, "encoding", *encoding)
	addQuery(params, "min_entropy", *minEntropy)
	addQuery(params, "max_entropy", *maxEntropy)
	if *limit != 0 {
		params.Set("limit", strconv.Itoa(*limit))
	}
	endpoint := "/samples/" + url.PathEscape(sha256) + "/strings"
	if *all {
		return a.getAllNestedCursor(context.Background(), endpoint, params, nestedCursorPager{itemsKey: "data", cursorParam: "cursor"})
	}
	return a.requestJSON(context.Background(), http.MethodGet, endpoint, params, nil, "", nil)
}
