# recondns

`recondns` is a small Go CLI for subdomain enumeration.

It combines:

- `subfinder`
- `chaos`
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
- `rapiddns-cli`

## Notes

- `subfinder` uses `-dL` internally for batch mode
- `chaos` uses `-dL` for batch mode and silently skips empty/no-result cases
- `rapiddns-cli` is queried once per root domain and results are merged
- output is deduplicated and normalized
- `-notify` is optional if you want a Feishu message after completion
- Chaos has a built-in default API key; set `CHAOS_KEY` or `PDCP_API_KEY` only if you want to override it
