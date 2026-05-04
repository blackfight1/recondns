# recondns

`recondns` is a small Go CLI for subdomain enumeration.

It combines:

- `subfinder`
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
- `rapiddns-cli`

## Notes

- `subfinder` uses `-dL` internally for batch mode
- `rapiddns-cli` is queried once per root domain and results are merged
- output is deduplicated and normalized
- `-notify` is optional if you want a Feishu message after completion
