# recondns

`recondns` is a small Go backend for:

- reading root domains from a file
- collecting subdomains with `subfinder`, `bbot`, and `rapiddns-cli`
- probing live URLs with `httpx`
- storing subdomains and live URLs in PostgreSQL
- running jobs in the background through a persistent DB-backed queue
- sending Feishu notifications with the same `FEISHU_WEBHOOK` env var used by your other project

## Features

- Persistent jobs in PostgreSQL
- CLI submit/run/export workflow
- Background worker mode that survives SSH disconnects when started with `nohup` or `systemd`
- Upsert-based asset storage with `first_seen_at` / `last_seen_at`
- Feishu notifications for queued / started / finished jobs

## Environment

Copy `.env.example` values into your shell or service environment:

```bash
export RECONDNS_DB_DSN="postgres://bbscope:CHANGE_ME@localhost:5432/bbscope?sslmode=disable"
export FEISHU_WEBHOOK="https://open.feishu.cn/open-apis/bot/v2/hook/0ef53dc3-91cf-43f2-abd5-8dc4d94c7b63"
```

Recommended:

- reuse the same PostgreSQL instance as `bbscope`
- keep `recondns` tables separate from `bbscope` tables

Tables created:

- `recon_jobs`
- `recon_root_domains`
- `recon_subdomains`
- `recon_web_endpoints`

## Build

```bash
cd /root/recondns
go build -o recondns ./cmd/recondns
```

## Usage

### 1. Submit a job

```bash
./recondns submit --input h1.txt --source h1
```

This only creates a queued job in the database.

### 2. Start a worker

```bash
./recondns worker --worker-id recon-worker-1
```

The worker continuously polls the DB for queued jobs and processes them.

### 3. Run a job immediately

```bash
./recondns run --input h1.txt --source h1
```

This submits and processes one job in the foreground.

### 4. List jobs

```bash
./recondns jobs --limit 20
```

### 5. Export stored assets

```bash
./recondns export subdomains --source h1 --output subs.txt
./recondns export urls --source h1 --output live_urls.txt
```

## Background operation

If you run this on your Ubuntu VPS and want it to keep working after SSH disconnects, use `screen`, `nohup`, or `systemd`.

### Ubuntu with `screen`

Create a screen session:

```bash
screen -S recondns-worker
```

Start the worker inside it:

```bash
cd /root/recondns
export RECONDNS_DB_DSN="postgres://bbscope:CHANGE_ME@localhost:5432/bbscope?sslmode=disable"
export FEISHU_WEBHOOK="https://open.feishu.cn/open-apis/bot/v2/hook/0ef53dc3-91cf-43f2-abd5-8dc4d94c7b63"
./recondns worker --worker-id recon-worker-1
```

Detach from screen without stopping the worker:

```bash
Ctrl+A, then D
```

Reattach later:

```bash
screen -r recondns-worker
```

### Linux with `nohup`

```bash
cd /root/recondns
nohup ./recondns worker --worker-id recon-worker-1 > /root/recondns-worker.log 2>&1 &
```

Submit jobs separately:

```bash
cd /root/recondns
./recondns submit --input /root/h1.txt --source h1
```

Check logs:

```bash
tail -f /root/recondns-worker.log
```

## Required tools

These binaries must be installed and available in `PATH`:

- `subfinder`
- `bbot`
- `rapiddns-cli`
- `httpx`

## Notes

- `subfinder`, `bbot`, and `rapiddns-cli` are run together and results are merged
- `httpx` warnings do not discard already discovered live URLs
- the worker is intentionally simple: one process can handle jobs continuously; you can scale later if needed
- on your VPS, `screen` is a practical default if you want to watch logs interactively while keeping the worker alive after SSH disconnects
