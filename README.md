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
redscli bulk search --field sha256 --values-file hashes.txt
redscli bulk fields
```

Use `--format pretty|json|raw|ndjson` for output control. `--all` follows supported cursor or offset pagination and combines returned item arrays.

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
