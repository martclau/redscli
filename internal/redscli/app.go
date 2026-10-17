package redscli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const version = "0.1.0"

// globalOptions stores options that apply before command dispatch.
type globalOptions struct {
	token      string
	tokenSet   bool
	baseURL    string
	baseURLSet bool
	timeout    time.Duration
	timeoutSet bool
	configPath string
	profile    string
	format     string
	verbose    bool
	version    bool
}

// app carries process IO, parsed global options, and the lazily constructed API
// client used by command handlers.
type app struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	opts   globalOptions
	client *client
}

// Main runs the CLI and returns a process exit code.
//
// It accepts explicit streams so tests can exercise command behavior without
// mutating os.Stdin, os.Stdout, or os.Stderr.
func Main(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	opts, rest, err := parseGlobal(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return exitCode(err)
	}
	if opts.version {
		fmt.Fprintf(out, "redscli %s\n", version)
		return 0
	}

	a := &app{in: in, out: out, errOut: errOut, opts: opts}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" || rest[0] == "--help" {
		a.printRootHelp()
		return 0
	}

	if err := a.run(rest); err != nil {
		a.printError(err)
		return exitCode(err)
	}
	return 0
}

// parseGlobal consumes only leading global options and returns the remaining
// command arguments unchanged.
//
// This intentionally stops at the first non-option so command-specific flags can
// be parsed by the command's own FlagSet.
func parseGlobal(args []string) (globalOptions, []string, error) {
	opts := globalOptions{
		baseURL:    defaultBaseURL,
		timeout:    60 * time.Second,
		configPath: defaultConfigPath(),
		format:     "pretty",
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			rest = append(rest, args[i:]...)
			break
		}
		name, value, hasValue := strings.Cut(arg, "=")
		needValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			i++
			if i >= len(args) {
				return "", withExitCode(2, "%s requires a value", name)
			}
			return args[i], nil
		}
		switch name {
		case "--token":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			opts.token = value
			opts.tokenSet = true
		case "--base-url":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			opts.baseURL = value
			opts.baseURLSet = true
		case "--timeout":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return opts, nil, withExitCode(2, "invalid --timeout: %v", err)
			}
			opts.timeout = parsed
			opts.timeoutSet = true
		case "--config":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			opts.configPath = value
		case "--profile":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			opts.profile = value
		case "--format":
			value, err := needValue()
			if err != nil {
				return opts, nil, err
			}
			if !validOutputFormat(value) {
				return opts, nil, withExitCode(2, "invalid --format %q; use pretty, json, raw, or ndjson", value)
			}
			opts.format = value
		case "--verbose":
			opts.verbose = true
		case "--version":
			opts.version = true
		case "-h", "--help":
			rest = []string{"help"}
			return opts, rest, nil
		default:
			return opts, nil, withExitCode(2, "unknown global option %s", name)
		}
	}
	return opts, rest, nil
}

// validOutputFormat reports whether a format is supported by writeBody.
func validOutputFormat(value string) bool {
	switch value {
	case "pretty", "json", "raw", "ndjson":
		return true
	default:
		return false
	}
}

// run dispatches the top-level command to its handler.
func (a *app) run(args []string) error {
	switch args[0] {
	case "search":
		return a.runSearch(args[1:])
	case "file", "sample":
		return a.runFile(args[1:])
	case "analyze":
		return a.runAnalyze(args[1:])
	case "collections":
		return a.runCollections(args[1:])
	case "fields":
		return a.runFields(args[1:])
	case "stats":
		return a.runSimpleGET(args[1:], "stats", "/stats")
	case "download":
		return a.runDownload(args[1:])
	case "quota":
		return a.runSimpleGET(args[1:], "quota", "/quota")
	case "upload":
		return a.runUpload(args[1:])
	case "job":
		return a.runJob(args[1:])
	case "jobs":
		return a.runJobs(args[1:])
	case "queue":
		return a.runSimpleGET(args[1:], "queue", "/queue/status")
	case "code":
		return a.runCode(args[1:])
	case "strings":
		return a.runStrings(args[1:])
	case "iocs":
		return a.runIOCs(args[1:])
	case "bulk":
		return a.runBulk(args[1:])
	case "raw":
		return a.runRaw(args[1:])
	case "config":
		return a.runConfig(args[1:])
	default:
		return withExitCode(2, "unknown command %q", args[0])
	}
}

// apiClient builds the authenticated HTTP client on first use.
//
// Config loading is delayed so non-API commands like help, version, and config
// edits do not require a token.
func (a *app) apiClient() (*client, error) {
	if a.client != nil {
		return a.client, nil
	}
	baseURL, token, timeout, err := effectiveSettings(a.opts)
	if err != nil {
		return nil, err
	}
	c, err := newClient(baseURL, token, timeout, a.opts.verbose, a.errOut)
	if err != nil {
		return nil, err
	}
	a.client = c
	return c, nil
}

// requestJSON executes an API request and writes the response using the selected
// output format.
func (a *app) requestJSON(ctx context.Context, method string, endpoint string, query url.Values, body io.Reader, contentType string, headers http.Header) error {
	c, err := a.apiClient()
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, method, endpoint, query, body, contentType, headers)
	if err != nil {
		return err
	}
	return writeBody(a.out, resp.body, a.opts.format)
}

// runSimpleGET handles commands whose API surface is exactly one GET endpoint.
func (a *app) runSimpleGET(args []string, name string, endpoint string) error {
	if hasHelp(args) {
		a.printCommandHelp(name)
		return nil
	}
	if len(args) != 0 {
		return withExitCode(2, "%s does not accept positional arguments", name)
	}
	return a.requestJSON(context.Background(), http.MethodGet, endpoint, nil, nil, "", nil)
}

// newFlagSet creates quiet flag sets so parse errors can be rendered uniformly
// by the CLI instead of the standard flag package.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// hasHelp checks for command-level help flags before normal parsing.
func hasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

// parseFlags parses command flags after normalizing interspersed flags.
func parseFlags(fs *flag.FlagSet, args []string) error {
	normalized, err := intersperseFlags(fs, args)
	if err != nil {
		return err
	}
	if err := fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return withExitCode(2, "%v", err)
	}
	return nil
}

// intersperseFlags allows users to put command flags after positional
// arguments, which Go's standard flag package does not support by default.
func intersperseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		nameValue := strings.TrimLeft(arg, "-")
		name, _, hasInlineValue := strings.Cut(nameValue, "=")
		f := fs.Lookup(name)
		if f == nil {
			flags = append(flags, arg)
			continue
		}
		flags = append(flags, arg)
		if !hasInlineValue && !isBoolFlag(f) {
			i++
			if i >= len(args) {
				return nil, withExitCode(2, "%s requires a value", arg)
			}
			flags = append(flags, args[i])
		}
	}
	return append(flags, positionals...), nil
}

// boolFlag is the optional interface used by flag.Value implementations that do
// not consume a following argument.
type boolFlag interface {
	IsBoolFlag() bool
}

// isBoolFlag reports whether a parsed flag is boolean-like.
func isBoolFlag(f *flag.Flag) bool {
	value, ok := f.Value.(boolFlag)
	return ok && value.IsBoolFlag()
}

// requireNoArgs enforces commands that should not receive positional arguments.
func requireNoArgs(fs *flag.FlagSet, command string) error {
	if fs.NArg() != 0 {
		return withExitCode(2, "%s does not accept positional arguments: %s", command, strings.Join(fs.Args(), " "))
	}
	return nil
}

// requireOneArg returns the single positional argument required by a command.
func requireOneArg(fs *flag.FlagSet, command string, name string) (string, error) {
	if fs.NArg() != 1 {
		return "", withExitCode(2, "%s requires exactly one <%s>", command, name)
	}
	return fs.Arg(0), nil
}

// printError renders API errors with their structured response body when
// available, while keeping local errors concise.
func (a *app) printError(err error) {
	var he *httpError
	if errors.As(err, &he) {
		fmt.Fprintf(a.errOut, "API error: %s\n", he.status)
		if len(he.body) > 0 {
			_ = writeBody(a.errOut, he.body, "pretty")
		}
		return
	}
	fmt.Fprintln(a.errOut, err)
}

// printRootHelp writes the top-level command summary.
func (a *app) printRootHelp() {
	fmt.Fprint(a.out, `redscli - REDS API command line client

Usage:
  redscli [global options] <command> [options]

Global options:
  --token <token>       API token. Defaults to REDS_API_TOKEN or config.
  --base-url <url>      API base URL. Defaults to https://reds.rationaledge.io/api/v1.
  --timeout <duration>  HTTP timeout. Defaults to 60s.
  --config <path>       Config file path.
  --profile <name>      Config profile. Defaults to default.
  --format <format>     Output format: pretty, json, raw, ndjson.
  --verbose             Print HTTP requests to stderr.
  --version             Print version.

Commands:
  search                Search across all collections.
  file                  Get sample metadata by MD5, SHA1, or SHA256.
  analyze               Queue sample or code analysis.
  collections           List available collections.
  fields                List searchable fields.
  stats                 Show dataset statistics.
  download              Download a sample ZIP or raw bytes.
  upload                Upload a malware sample.
  quota                 Show current quota usage.
  job, jobs             Get one job or list jobs.
  queue                 Show analysis queue status.
  code                  Search and retrieve code analysis data.
  strings               Search and retrieve string data.
  iocs                  Retrieve sample IOC summaries and details.
  bulk                  Bulk exact-value search.
  raw                   Make a direct authenticated API request.
  config                Manage local profiles.

Examples:
  redscli search -q 'filetype_magika="pebin" +filesize:>100000'
  redscli file 0f5409a5df1f916fe532696746a46d92cb561d981953d9ec30712d6d4670a13f
  redscli upload sample.exe --private
  redscli code search -q 'decompiled_function:"CreateFile"'
  redscli strings samples --string 'GetProcAddress' --limit 50
  redscli bulk search --field sha256 --values-file hashes.txt

Run redscli <command> --help for command-specific options.
`)
}

// printCommandHelp writes detailed help for a single command group.
func (a *app) printCommandHelp(name string) {
	help, ok := commandHelp[name]
	if !ok {
		fmt.Fprintf(a.out, "No detailed help for %s.\n", name)
		return
	}
	fmt.Fprint(a.out, help)
}

// commandHelp contains static help text for command groups.
var commandHelp = map[string]string{
	"search": `Usage:
  redscli search -q <query> [--cursor <cursor>] [--fields <csv>] [--api-format json|ndjson] [--all]

Search query examples:
  redscli search -q 'compiler:"Visual Basic"'
  redscli search -q 'has_overlay=1 -language:"go"'
`,
	"file": `Usage:
  redscli file <md5|sha1|sha256>

Retrieves full sample details when analyzed, or catalog-only status for archived samples.
`,
	"analyze": `Usage:
  redscli analyze sample <sha256>
  redscli analyze code <sha256>
`,
	"collections": `Usage:
  redscli collections
`,
	"fields": `Usage:
  redscli fields [--collection <name>]
`,
	"stats": `Usage:
  redscli stats
`,
	"quota": `Usage:
  redscli quota
`,
	"queue": `Usage:
  redscli queue
`,
	"download": `Usage:
  redscli download <hash> [--output <path>] [--stdout] [--raw]

By default, samples are returned as ZIP files. The ZIP password is: infected
Use --raw to download plaintext sample bytes from /download/{hash}/raw.
`,
	"upload": `Usage:
  redscli upload <path> [--filename <name>] [--private]

--filename sets the multipart upload filename without renaming the local file.
By default, the local file's basename is used.
`,
	"job": `Usage:
  redscli job <job_id>
`,
	"jobs": `Usage:
  redscli jobs [--status queued|running|completed|failed] [--limit <n>] [--offset <n>] [--all]

--all combines every page from the starting --offset. --limit sets the page size.
`,
	"code": `Usage:
  redscli code search -q <query> [--cursor <cursor>] [--all]
  redscli code validation <sha256>
  redscli code decompiled <sha256> [--cursor <cursor>] [--limit <n>] [--all]
  redscli code disassembled <sha256> [--cursor <cursor>] [--limit <n>] [--all]
  redscli code stats <sha256>
`,
	"strings": `Usage:
  redscli strings search -q <query> [--cursor <cursor>] [--all]
  redscli strings samples --string <value> [--offset <n>] [--limit <n>] [--all]
  redscli strings stats <sha256>
  redscli strings list <sha256> [--cursor <cursor>] [--limit <n>] [--search <term>] [--encoding <type>] [--min-entropy <n>] [--max-entropy <n>] [--all]
`,
	"iocs": `Usage:
  redscli iocs summary <sha256>
  redscli iocs list <sha256> [--include-counts=false] [--types ipv4,url,fqdn]
`,
	"bulk": `Usage:
  redscli bulk search --field <field> (--value <v>... | --values-file <path>) [--cursor <cursor>] [--all]
  redscli bulk fields

--all combines every result page from the starting cursor. The input limit
remains 300 values, and each page request consumes API quota.
`,
	"raw": `Usage:
  redscli raw <METHOD> <path> [--query key=value]... [--header 'Name: value']... [--body-json <json|@file>] [--body-file <path>]
`,
	"config": `Usage:
  redscli config init [--token <token>] [--base-url <url>] [--profile <name>]
  redscli config set token <token> [--profile <name>]
  redscli config set base-url <url> [--profile <name>]
  redscli config show [--redact=false]
`,
}
