package redscli

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (a *app) runCode(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("code")
		return nil
	}
	switch args[0] {
	case "search":
		return a.runCodeSearch(args[1:])
	case "validation", "validate":
		return a.runCodeSampleGET("code validation", "/samples/%s/validation", args[1:])
	case "decompiled":
		return a.runFunctions("code decompiled", "/samples/%s/functions/decompiled", args[1:])
	case "disassembled":
		return a.runFunctions("code disassembled", "/samples/%s/functions/disassembled", args[1:])
	case "stats", "statistics":
		return a.runCodeSampleGET("code stats", "/samples/%s/code-statistics", args[1:])
	default:
		return withExitCode(2, "unknown code subcommand %q", args[0])
	}
}

func (a *app) runCodeSearch(args []string) error {
	fs := newFlagSet("code search")
	query := fs.String("q", "", "code-domain query")
	fs.StringVar(query, "query", "", "code-domain query")
	cursor := fs.String("cursor", "", "pagination cursor")
	all := fs.Bool("all", false, "follow cursor pagination and combine results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *query == "" && fs.NArg() == 1 {
		*query = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return withExitCode(2, "code search accepts at most one positional query")
	}
	if strings.TrimSpace(*query) == "" {
		return withExitCode(2, "code search requires -q <query>")
	}
	params := url.Values{"q": []string{*query}}
	addQuery(params, "cursor", *cursor)
	if *all {
		return a.getAllCursor(context.Background(), "/code/search", params, cursorPager{itemsKey: "results", cursorParam: "cursor"})
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/code/search", params, nil, "", nil)
}

func (a *app) runCodeSampleGET(command string, pattern string, args []string) error {
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

func (a *app) runFunctions(command string, pattern string, args []string) error {
	fs := newFlagSet(command)
	cursor := fs.String("cursor", "", "pagination cursor")
	limit := fs.Int("limit", 0, "number of functions, 1-100")
	all := fs.Bool("all", false, "follow cursor pagination and combine data")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, command, "sha256")
	if err != nil {
		return err
	}
	params := url.Values{}
	addQuery(params, "cursor", *cursor)
	if *limit != 0 {
		params.Set("limit", strconv.Itoa(*limit))
	}
	endpoint := strings.Replace(pattern, "%s", url.PathEscape(sha256), 1)
	if *all {
		return a.getAllNestedCursor(context.Background(), endpoint, params, nestedCursorPager{itemsKey: "data", cursorParam: "cursor"})
	}
	return a.requestJSON(context.Background(), http.MethodGet, endpoint, params, nil, "", nil)
}
