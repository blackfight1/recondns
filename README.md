# recondns

`recondns` is a small Go CLI for subdomain enumeration.

It combines:

- `subfinder`
- `chaos`
- `assetfinder`
- `findomain`
- `rapiddns-cli`

and outputs a clean subdomain list.

## Usage

Single domain:

```bash
recondns -d hackerone.com -o h1-subs.txt
```

Batch input:

```bash
recondns -dL h1.txt -o h1-subs.txt
```

JSON output:

```bash
recondns -d hackerone.com -json
recondns -dL h1.txt -json -o h1-subs.json
```

Without `-o`, results are printed to stdout:

```bash
recondns -d hackerone.com
recondns -dL h1.txt
```

## Input file format

One root domain per line:

```txt
hackerone.com
bugcrowd.com
example.com
```

Empty lines and lines starting with `#` are ignored.

## Build

```bash
cd /root/recondns
go build -o recondns ./cmd/recondns
```

## Required tools

These binaries must be available in `PATH`:

- `subfinder`
- `chaos`
- `assetfinder`
- `findomain`
- `rapiddns-cli`

## Notes

- `subfinder` uses `-dL` internally for batch mode
- `chaos` uses `-dL` for batch mode and silently skips empty/no-result cases
- `assetfinder` uses `assetfinder --subs-only <domain>` and batch mode is implemented as one-by-one execution because upstream does not provide native file/stdin batch input yet
- `findomain` uses the official file input mode (`-f`) for batch runs and `-t` for single targets
- `rapiddns-cli` is queried once per root domain and results are merged
- output is deduplicated and normalized
- `-notify` is optional if you want a Feishu message after completion
- Chaos has a built-in default API key; set `CHAOS_KEY` or `PDCP_API_KEY` only if you want to override it
- current version is a pure CLI enumerator and does not depend on PostgreSQL
