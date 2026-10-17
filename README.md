# redscli

`redscli` is a dependency-free Go CLI for the RationalEdge DataSet (REDS) API v1 defined in `docs/reds-api-v1-openapi.html`.

## Build

```sh
go build -buildvcs=false ./cmd/redscli
```

## Authentication

Every REDS endpoint requires a bearer token. Use one of these:

```sh
export REDS_API_TOKEN=your_token
redscli config set token your_token
redscli --token your_token quota
```

The default API base URL is `https://reds.rationaledge.io/api/v1`. Override it with `--base-url`, `REDS_API_BASE_URL`, or `redscli config set base-url <url>`.

## Common Usage

```sh
redscli search -q 'filetype_magika="pebin" +filesize:>100000'
redscli file 0f5409a5df1f916fe532696746a46d92cb561d981953d9ec30712d6d4670a13f
redscli analyze sample 7b864591a77a15197d9f25ed3e625b50576ffc061f2849ac6fcc245d296b7357
redscli upload sample.exe --private
redscli download 0f5409a5df1f916fe532696746a46d92cb561d981953d9ec30712d6d4670a13f --output sample.zip
redscli download 0f5409a5df1f916fe532696746a46d92cb561d981953d9ec30712d6d4670a13f --raw --output sample.bin
redscli jobs --status completed --limit 10
```

Code, strings, IOC, and bulk endpoints are grouped:

```sh
redscli code search -q 'decompiled_function:"CreateFile"'
redscli code decompiled <sha256> --limit 100 --all
redscli strings search -q 'string:"password"'
redscli strings samples --string 'GetProcAddress' --limit 50
redscli strings list <sha256> --search http --min-entropy 4.0
redscli iocs summary <sha256>
redscli iocs list <sha256> --types ipv4,ipv6,fqdn,url,server
redscli bulk search --field sha256 --values-file hashes.txt --all
redscli bulk fields
redscli jobs --status completed --limit 50 --all
```

Use `--format pretty|json|raw|ndjson` for output control. `--all` follows supported cursor or offset pagination and combines returned item arrays.

For `bulk search --all`, each page repeats the same field and input values with
the next cursor. The maximum input remains 300 values. For `jobs --all`, `--limit`
sets the page size and `--offset` sets the starting position. Both commands add a
`pages` count to the combined response and preserve other first-page metadata.
Jobs output reports the starting offset and the combined job count as `limit`.
Every page is a separate API request and uses the applicable quota. An error on
any page fails the command without emitting a partial combined response.

## Upload Filenames

Use `--filename` to send an original filename while keeping the local sample named
by its SHA-256:

```sh
./redscli upload downloads/samples/<sha256>.bin --filename original-sample.exe
```

The name is sent in the multipart file header. Without `--filename`, the CLI uses
the local basename. Supply a name without directory components, newlines, or NUL
bytes. MalwareBazaar manifests contain the original name in each sample's
`file_name` field. The bundled REDS API reference documents the file and `private`
flag for uploads; it does not document arbitrary tags or custom metadata fields.

## Download MalwareBazaar Samples by Tag

The standalone script requires `curl`, `jq`, `sha256sum`, and `7z` or `7zz`
with ZIP/AES support. Get an Auth-Key from <https://auth.abuse.ch/>.

```sh
./scripts/download-malwarebazaar.sh --tag DDosia --limit 100
```

The default destination is the repository's `downloads/` directory, regardless of
the current working directory. This directory is ignored by Git. All tags share
the same sample store, so a sample already downloaded for another tag is reused.
Use `--output DIRECTORY` to choose a different destination.

The script prompts for the key without displaying it, or reads `MB_AUTH_KEY`
when set. It keeps encrypted ZIPs in `downloads/archives/`, unpacked samples in
`downloads/samples/`, and a separate manifest for each run under
`downloads/manifests/`, for example `downloads/manifests/20261017T142530Z.a1B2c3/`.
The UTC timestamp identifies the run start;
the random suffix prevents collisions between runs started in the same second.
Each manifest contains:

- `tag.json`: the original API response, including each sample's tags and metadata.
- `run.json`: the queried tag, limit, start and finish times, completion status,
  exit code, and counts of downloaded and skipped samples.
- `downloads.ndjson`: one record per verified sample, with its SHA-256, download
  or skip status, verification time, and download completion time for new files.

Completed records are retained if a later download fails. Every run gets a new
manifest, including empty or failed queries; `tag.json` is present when a query
response was received successfully. Earlier manifests and any legacy `tag.json`
at the output root are left intact.

Each sample's SHA-256 is verified before saving; reruns skip samples whose hashes
still match. Custom destinations inside the repository need their own ignore
rule. The script stops on the first error and does not upload samples to REDS.

The default limit is 1,000. MalwareBazaar's [tag API](https://bazaar.abuse.ch/api/#tag)
returns at most 1,000 samples with no documented pagination, so a selection may
omit historical matches. The [file download limit](https://bazaar.abuse.ch/faq/#limit)
is 2,000 samples per IP per day. Downloads use ZIP password `infected`.

## Raw Requests

Use `raw` when the API grows before the CLI does:

```sh
redscli raw GET /quota
redscli raw POST /bulk-search --body-json '{"field":"ipv4","values":["8.8.8.8"]}'
redscli raw GET /search --query 'q=compiler:"Visual Basic"'
```

## Exit Codes

- `0`: success
- `1`: local runtime error
- `2`: usage or configuration error
- `3`: API error
- `4`: unauthorized
- `5`: forbidden
- `6`: not found
- `7`: rate limited
