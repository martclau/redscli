package redscli

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runSearch handles the general /search endpoint.
func (a *app) runSearch(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("search")
		return nil
	}
	fs := newFlagSet("search")
	query := fs.String("q", "", "search query")
	fs.StringVar(query, "query", "", "search query")
	cursor := fs.String("cursor", "", "pagination cursor")
	fields := fs.String("fields", "", "comma-separated fields to include")
	apiFormat := fs.String("api-format", "", "API response format: json or ndjson")
	all := fs.Bool("all", false, "follow cursor pagination and combine results")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *query == "" && fs.NArg() == 1 {
		*query = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return withExitCode(2, "search accepts at most one positional query")
	}
	if strings.TrimSpace(*query) == "" {
		return withExitCode(2, "search requires -q <query>")
	}

	params := url.Values{"q": []string{*query}}
	addQuery(params, "cursor", *cursor)
	addQuery(params, "fields", *fields)
	addQuery(params, "format", *apiFormat)
	if *all {
		// The pagination combiner must parse JSON; NDJSON is intentionally
		// streamed as-is by the API and cannot be merged safely.
		if *apiFormat == "ndjson" {
			return withExitCode(2, "search --all requires JSON API responses; omit --api-format ndjson")
		}
		return a.getAllCursor(context.Background(), "/search", params, cursorPager{itemsKey: "results", cursorParam: "cursor"})
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/search", params, nil, "", nil)
}

// runFile retrieves sample metadata by MD5, SHA1, or SHA256.
func (a *app) runFile(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("file")
		return nil
	}
	fs := newFlagSet("file")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	hash, err := requireOneArg(fs, "file", "hash")
	if err != nil {
		return err
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/file/"+url.PathEscape(hash), nil, nil, "", nil)
}

// runAnalyze dispatches analysis subcommands for full sample or code-only
// analysis.
func (a *app) runAnalyze(args []string) error {
	if len(args) == 0 || hasHelp(args) {
		a.printCommandHelp("analyze")
		return nil
	}
	switch args[0] {
	case "sample":
		return a.runAnalyzePost("analyze sample", "/analyze-sample/", args[1:])
	case "code":
		return a.runAnalyzePost("analyze code", "/analyze-code/", args[1:])
	default:
		return withExitCode(2, "unknown analyze subcommand %q", args[0])
	}
}

// runAnalyzePost posts to one analysis endpoint after validating the SHA256
// positional argument.
func (a *app) runAnalyzePost(command string, endpointPrefix string, args []string) error {
	fs := newFlagSet(command)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	sha256, err := requireOneArg(fs, command, "sha256")
	if err != nil {
		return err
	}
	return a.requestJSON(context.Background(), http.MethodPost, endpointPrefix+url.PathEscape(sha256), nil, nil, "", nil)
}

// runCollections lists REDS collections.
func (a *app) runCollections(args []string) error {
	return a.runSimpleGET(args, "collections", "/collections")
}

// runFields lists searchable field mappings, optionally scoped to a collection.
func (a *app) runFields(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("fields")
		return nil
	}
	fs := newFlagSet("fields")
	collection := fs.String("collection", "", "collection name")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := requireNoArgs(fs, "fields"); err != nil {
		return err
	}
	params := url.Values{}
	addQuery(params, "collection", *collection)
	return a.requestJSON(context.Background(), http.MethodGet, "/fields", params, nil, "", nil)
}

// runDownload streams a sample ZIP or raw bytes to a file or stdout.
func (a *app) runDownload(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("download")
		return nil
	}
	fs := newFlagSet("download")
	output := fs.String("output", "", "output file path")
	stdout := fs.Bool("stdout", false, "write ZIP bytes to stdout")
	raw := fs.Bool("raw", false, "download raw sample bytes instead of ZIP")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	hash, err := requireOneArg(fs, "download", "hash")
	if err != nil {
		return err
	}

	c, err := a.apiClient()
	if err != nil {
		return err
	}
	endpoint := "/download/" + url.PathEscape(hash)
	fallbackName := hash + ".zip"
	if *raw {
		endpoint += "/raw"
		fallbackName = hash
	}
	resp, err := c.doStream(context.Background(), http.MethodGet, endpoint, nil, nil, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if *stdout {
		// Binary output must bypass JSON formatting entirely.
		_, err := io.Copy(a.out, resp.Body)
		return err
	}

	target := *output
	if target == "" {
		target = downloadFilename(resp.Header, fallbackName)
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	bytes, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintf(a.out, "saved %s (%d bytes)\n", target, bytes)
	if !*raw {
		fmt.Fprintln(a.out, "zip password: infected")
	}
	return nil
}

// downloadFilename chooses the server-provided filename when present and falls
// back to a deterministic name.
func downloadFilename(header http.Header, fallback string) string {
	disposition := header.Get("Content-Disposition")
	if disposition != "" {
		_, params, err := mime.ParseMediaType(disposition)
		if err == nil {
			if filename := strings.TrimSpace(params["filename"]); filename != "" {
				return filepath.Base(filename)
			}
		}
	}
	return fallback
}

// runUpload streams one file as multipart/form-data to the upload endpoint.
func (a *app) runUpload(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("upload")
		return nil
	}
	fs := newFlagSet("upload")
	private := fs.Bool("private", false, "mark upload private")
	uploadName := fs.String("filename", "", "filename to send instead of the local basename")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	filename, err := requireOneArg(fs, "upload", "path")
	if err != nil {
		return err
	}
	submittedName := *uploadName
	if submittedName == "" {
		submittedName = filepath.Base(filename)
	}
	if submittedName == "." || submittedName == ".." || strings.ContainsAny(submittedName, "/\\\r\n\x00") {
		return withExitCode(2, "upload filename must be a name without path separators, newlines, or NUL bytes")
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Stat(); err != nil {
		return err
	}

	bodyReader, bodyWriter := io.Pipe()
	writer := multipart.NewWriter(bodyWriter)
	go func() {
		defer bodyWriter.Close()
		// The pipe avoids buffering potentially large malware samples in memory.
		part, err := writer.CreateFormFile("file", submittedName)
		if err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			bodyWriter.CloseWithError(err)
			return
		}
		if *private {
			if err := writer.WriteField("private", "true"); err != nil {
				bodyWriter.CloseWithError(err)
				return
			}
		}
		if err := writer.Close(); err != nil {
			bodyWriter.CloseWithError(err)
		}
	}()

	return a.requestJSON(context.Background(), http.MethodPost, "/upload", nil, bodyReader, writer.FormDataContentType(), nil)
}

// runJob retrieves one analysis job by ID.
func (a *app) runJob(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("job")
		return nil
	}
	fs := newFlagSet("job")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	jobID, err := requireOneArg(fs, "job", "job_id")
	if err != nil {
		return err
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/job/"+url.PathEscape(jobID), nil, nil, "", nil)
}

// runJobs lists analysis jobs with optional status and offset pagination
// filters.
func (a *app) runJobs(args []string) error {
	if hasHelp(args) {
		a.printCommandHelp("jobs")
		return nil
	}
	fs := newFlagSet("jobs")
	status := fs.String("status", "", "job status")
	limit := fs.Int("limit", 50, "maximum jobs")
	offset := fs.Int("offset", 0, "pagination offset")
	all := fs.Bool("all", false, "follow offset pagination and combine jobs")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := requireNoArgs(fs, "jobs"); err != nil {
		return err
	}
	if *limit <= 0 || *offset < 0 {
		return withExitCode(2, "jobs requires --limit greater than zero and --offset zero or greater")
	}
	params := url.Values{}
	addQuery(params, "status", *status)
	if *limit != 50 {
		params.Set("limit", strconv.Itoa(*limit))
	}
	if *offset != 0 {
		params.Set("offset", strconv.Itoa(*offset))
	}
	if *all {
		return a.getAllJobs(context.Background(), params, *offset)
	}
	return a.requestJSON(context.Background(), http.MethodGet, "/jobs", params, nil, "", nil)
}

// addQuery sets a query parameter only when the user supplied a non-empty value.
func addQuery(params url.Values, key, value string) {
	if strings.TrimSpace(value) != "" {
		params.Set(key, value)
	}
}
