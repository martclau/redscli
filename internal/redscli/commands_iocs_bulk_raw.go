package redscli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// runIOCs dispatches IOC summary and detail commands.
func (a *app) runIOCs(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("iocs")
		return nil
	}
	switch args[0] {
	case "summary":
		return a.runIOCSummary(args[1:])
	case "list":
		return a.runIOCList(args[1:])
	default:
		return withExitCode(2, "unknown iocs subcommand %q", args[0])
	}
}

// runIOCSummary retrieves per-type IOC counts for one sample.
func (a *app) runIOCSummary(args []string) error {
	fs := newFlagSet("iocs summary")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, "iocs summary", "sha256")
	if err != nil {
		return err
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/samples/"+url.PathEscape(sha256)+"/iocs/summary", nil, nil, "", nil)
}

// runIOCList retrieves grouped IOC values for one sample.
func (a *app) runIOCList(args []string) error {
	fs := newFlagSet("iocs list")
	includeCounts := fs.Bool("include-counts", true, "include occurrence counts")
	types := fs.String("types", "", "comma-separated IOC types")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, "iocs list", "sha256")
	if err != nil {
		return err
	}
	params := url.Values{}
	if !*includeCounts {
		params.Set("include_counts", "false")
	}
	addQuery(params, "ioc_types", *types)
	return a.requestJSON(context.Background(), http.MethodGet, "/samples/"+url.PathEscape(sha256)+"/iocs", params, nil, "", nil)
}

// runBulk dispatches bulk-search commands.
func (a *app) runBulk(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("bulk")
		return nil
	}
	switch args[0] {
	case "search":
		return a.runBulkSearch(args[1:])
	case "fields":
		return a.runSimpleGET(args[1:], "bulk", "/bulk-search/fields")
	default:
		return withExitCode(2, "unknown bulk subcommand %q", args[0])
	}
}

type bulkSearchRequest struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
	Cursor string   `json:"cursor,omitempty"`
}

// runBulkSearch posts exact values for one bulk-eligible REDS field.
func (a *app) runBulkSearch(args []string) error {
	fs := newFlagSet("bulk search")
	field := fs.String("field", "", "bulk-eligible field")
	valuesFile := fs.String("values-file", "", "file with one value per line")
	valuesCSV := fs.String("values", "", "comma-separated values")
	cursor := fs.String("cursor", "", "pagination cursor")
	all := fs.Bool("all", false, "follow cursor pagination and combine results")
	var values stringSlice
	fs.Var(&values, "value", "value to search; repeatable")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := requireNoArgs(fs, "bulk search"); err != nil {
		return err
	}
	if strings.TrimSpace(*field) == "" {
		return withExitCode(2, "bulk search requires --field <field>")
	}
	if *valuesCSV != "" {
		for _, value := range strings.Split(*valuesCSV, ",") {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				values = append(values, trimmed)
			}
		}
	}
	if *valuesFile != "" {
		fromFile, err := readValuesFile(*valuesFile)
		if err != nil {
			return err
		}
		values = append(values, fromFile...)
	}
	if len(values) == 0 {
		return withExitCode(2, "bulk search requires --value, --values, or --values-file")
	}
	if len(values) > 300 {
		// The API caps a bulk request at 300 values; fail locally before
		// spending quota on a request that cannot succeed.
		return withExitCode(2, "bulk search accepts at most 300 values per request")
	}
	payload := bulkSearchRequest{
		Field:  *field,
		Values: []string(values),
		Cursor: *cursor,
	}
	if *all {
		return a.postAllBulkSearch(context.Background(), payload)
	}
	body, contentType, err := jsonBody(payload)
	if err != nil {
		return err
	}
	return a.requestJSON(context.Background(), http.MethodPost, "/bulk-search", nil, body, contentType, nil)
}

// runRaw performs an authenticated request to an arbitrary REDS API path.
//
// This keeps the CLI usable when the API grows before a first-class command is
// added.
func (a *app) runRaw(args []string) error {
	if hasHelp(args) || len(args) == 0 {
		a.printCommandHelp("raw")
		return nil
	}
	fs := newFlagSet("raw")
	var queries stringSlice
	var headersList stringSlice
	bodyJSON := fs.String("body-json", "", "JSON request body or @file")
	bodyFile := fs.String("body-file", "", "raw request body file")
	contentType := fs.String("content-type", "", "request content type")
	fs.Var(&queries, "query", "query key=value; repeatable")
	fs.Var(&headersList, "header", "header 'Name: value'; repeatable")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return withExitCode(2, "raw requires <METHOD> <path>")
	}
	method := strings.ToUpper(fs.Arg(0))
	endpoint := fs.Arg(1)

	params := url.Values{}
	for _, item := range queries {
		key, value, err := parseKeyValue(item)
		if err != nil {
			return err
		}
		params.Add(key, value)
	}

	headers := http.Header{}
	for _, item := range headersList {
		key, value, ok := strings.Cut(item, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid header %q; expected 'Name: value'", item)
		}
		headers.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}

	var body io.Reader
	ct := *contentType
	switch {
	case *bodyJSON != "" && *bodyFile != "":
		return withExitCode(2, "use only one of --body-json or --body-file")
	case *bodyJSON != "":
		data, err := readInlineOrFile(*bodyJSON)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
		if ct == "" {
			ct = "application/json"
		}
	case *bodyFile != "":
		file, err := os.Open(*bodyFile)
		if err != nil {
			return err
		}
		defer file.Close()
		body = file
	}
	return a.requestJSON(context.Background(), method, endpoint, params, body, ct, headers)
}

// readInlineOrFile supports --body-json '<json>' and --body-json @payload.json.
func readInlineOrFile(value string) ([]byte, error) {
	if strings.HasPrefix(value, "@") {
		return os.ReadFile(strings.TrimPrefix(value, "@"))
	}
	return []byte(value), nil
}
