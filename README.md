# recondns

Go CLI that runs **subfinder**, **chaos**, **assetfinder**, **findomain**, and **bbot** (passive) in parallel, then outputs a deduplicated subdomain list.

## Usage

```bash
# single domain
recondns -d hackerone.com -o h1-subs.txt

# domain list
recondns -dL domains.txt -o subs.txt

# JSON / stdout
recondns -d hackerone.com -json
recondns -d hackerone.com
```

| Flag | Description |
|------|-------------|
| `-d` | Single root domain |
| `-dL` | File with one root domain per line (`#` and empty lines ignored) |
| `-o` | Output file (default: stdout) |
| `-json` | JSON output (`roots` + `subdomains`) |

## Build

```bash
go build -o recondns ./cmd/recondns
```

## Requirements

These binaries must be on `PATH`:

- `subfinder`
- `chaos`
- `assetfinder`
- `findomain`
- `bbot` ([install](https://github.com/blacklanternsecurity/bbot))

Optional env: `CHAOS_KEY` or `PDCP_API_KEY` (overrides the built-in Chaos key).

### bbot

Uses passive subdomain enum only:

```bash
bbot -t <targets> -p subdomain-enum -rf passive -y
```

Results come from `subdomains.txt` (in-scope, resolved). API keys go in `~/.config/bbot/` as usual.
