package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"recondns/internal/model"
	"recondns/internal/normalize"
)

type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &Store{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SubmitJob(ctx context.Context, source, inputFile string, roots []string, notify bool) (model.ReconJob, error) {
	roots = normalize.Domains(roots)
	source = strings.TrimSpace(source)
	if source == "" {
		source = "default"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ReconJob{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var job model.ReconJob
	query := `
INSERT INTO recon_jobs (source, input_file, status, notify_enabled, root_domain_count)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, source, input_file, status, notify_enabled, root_domain_count, subdomain_count, live_url_count, error_message, worker_id, started_at, finished_at, created_at, updated_at`
	err = tx.QueryRowContext(ctx, query, source, inputFile, model.JobQueued, notify, len(roots)).Scan(
		&job.ID,
		&job.Source,
		&job.InputFile,
		&job.Status,
		&job.NotifyEnabled,
		&job.RootDomainCount,
		&job.SubdomainCount,
		&job.LiveURLCount,
		&job.ErrorMessage,
		&job.WorkerID,
		&job.StartedAt,
		&job.FinishedAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return model.ReconJob{}, err
	}

	for _, root := range roots {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO recon_root_domains (job_id, source, root_domain)
VALUES ($1, $2, $3)
ON CONFLICT (job_id, root_domain) DO NOTHING
`, job.ID, source, root); err != nil {
			return model.ReconJob{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return model.ReconJob{}, err
	}
	return job, nil
}

func (s *Store) LoadJob(ctx context.Context, jobID int64) (model.ReconJobWithRoots, error) {
	var job model.ReconJobWithRoots
	err := s.db.QueryRowContext(ctx, `
SELECT id, source, input_file, status, notify_enabled, worker_id, root_domain_count, subdomain_count, live_url_count, error_message, started_at, finished_at, created_at, updated_at
FROM recon_jobs
WHERE id = $1
`, jobID).Scan(
		&job.ID,
		&job.Source,
		&job.InputFile,
		&job.Status,
		&job.NotifyEnabled,
		&job.WorkerID,
		&job.RootDomainCount,
		&job.SubdomainCount,
		&job.LiveURLCount,
		&job.ErrorMessage,
		&job.StartedAt,
		&job.FinishedAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return job, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT root_domain
FROM recon_root_domains
WHERE job_id = $1
ORDER BY root_domain
`, jobID)
	if err != nil {
		return job, err
	}
	defer rows.Close()

	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return job, err
		}
		job.RootDomains = append(job.RootDomains, root)
	}
	return job, rows.Err()
}

func (s *Store) ClaimNextQueuedJob(ctx context.Context, workerID string) (model.ReconJobWithRoots, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ReconJobWithRoots{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var jobID int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM recon_jobs
WHERE status = $1
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1
`, model.JobQueued).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ReconJobWithRoots{}, false, nil
	}
	if err != nil {
		return model.ReconJobWithRoots{}, false, err
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
UPDATE recon_jobs
SET status = $1, worker_id = $2, started_at = $3, updated_at = $3, error_message = ''
WHERE id = $4
`, model.JobRunning, workerID, now, jobID); err != nil {
		return model.ReconJobWithRoots{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return model.ReconJobWithRoots{}, false, err
	}

	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		return model.ReconJobWithRoots{}, false, err
	}
	return job, true, nil
}

func (s *Store) MarkJobSucceeded(ctx context.Context, jobID int64, subdomainCount, liveURLCount int) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE recon_jobs
SET status = $1, subdomain_count = $2, live_url_count = $3, finished_at = $4, updated_at = $4
WHERE id = $5
`, model.JobSucceeded, subdomainCount, liveURLCount, now, jobID)
	return err
}

func (s *Store) MarkJobFailed(ctx context.Context, jobID int64, message string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
UPDATE recon_jobs
SET status = $1, error_message = $2, finished_at = $3, updated_at = $3
WHERE id = $4
`, model.JobFailed, truncate(message, 4000), now, jobID)
	return err
}

func (s *Store) UpsertSubdomains(ctx context.Context, job model.ReconJobWithRoots, items []model.SubdomainAsset) error {
	for _, item := range items {
		root := normalize.MatchRootDomain(item.Subdomain, job.RootDomains)
		if root == "" {
			root = normalize.Domain(item.RootDomain)
		}
		discoveredBy := strings.Join(uniqueStrings(item.DiscoveredBy), ",")
		_, err := s.db.ExecContext(ctx, `
INSERT INTO recon_subdomains (
	root_domain, subdomain, source, first_seen_at, last_seen_at, first_job_id, last_job_id, discovered_by
)
VALUES ($1, $2, $3, NOW(), NOW(), $4, $4, $5)
ON CONFLICT (root_domain, subdomain) DO UPDATE
SET last_seen_at = NOW(),
    last_job_id = EXCLUDED.last_job_id,
    source = EXCLUDED.source,
    discovered_by = CASE
        WHEN recon_subdomains.discovered_by = '' THEN EXCLUDED.discovered_by
        WHEN EXCLUDED.discovered_by = '' THEN recon_subdomains.discovered_by
        ELSE recon_subdomains.discovered_by || ',' || EXCLUDED.discovered_by
    END,
    updated_at = NOW()
`, root, normalize.Domain(item.Subdomain), job.Source, job.ID, truncate(discoveredBy, 500))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertWebEndpoints(ctx context.Context, job model.ReconJobWithRoots, items []model.WebEndpoint) error {
	for _, item := range items {
		root := normalize.MatchRootDomain(item.Subdomain, job.RootDomains)
		if root == "" {
			root = normalize.MatchRootDomain(item.Host, job.RootDomains)
		}
		techJSON, err := json.Marshal(uniqueStrings(item.Tech))
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `
INSERT INTO recon_web_endpoints (
	url, root_domain, subdomain, source, scheme, host, port, status_code, title, tech_json, ip, webserver, cdn, cdn_name, content_type, first_seen_at, last_seen_at, first_job_id, last_job_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW(), $16, $16)
ON CONFLICT (url) DO UPDATE
SET root_domain = EXCLUDED.root_domain,
    subdomain = EXCLUDED.subdomain,
    source = EXCLUDED.source,
    scheme = EXCLUDED.scheme,
    host = EXCLUDED.host,
    port = EXCLUDED.port,
    status_code = EXCLUDED.status_code,
    title = EXCLUDED.title,
    tech_json = EXCLUDED.tech_json,
    ip = EXCLUDED.ip,
    webserver = EXCLUDED.webserver,
    cdn = EXCLUDED.cdn,
    cdn_name = EXCLUDED.cdn_name,
    content_type = EXCLUDED.content_type,
    last_seen_at = NOW(),
    last_job_id = EXCLUDED.last_job_id,
    updated_at = NOW()
`, item.URL, root, normalize.Domain(item.Subdomain), job.Source, item.Scheme, normalize.Domain(item.Host), item.Port, item.StatusCode, truncate(item.Title, 2000), string(techJSON), item.IP, truncate(item.Webserver, 255), item.CDN, truncate(item.CDNName, 255), truncate(item.ContentType, 255), job.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]model.ReconJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source, input_file, status, notify_enabled, worker_id, root_domain_count, subdomain_count, live_url_count, error_message, started_at, finished_at, created_at, updated_at
FROM recon_jobs
ORDER BY id DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.ReconJob
	for rows.Next() {
		var job model.ReconJob
		if err := rows.Scan(
			&job.ID,
			&job.Source,
			&job.InputFile,
			&job.Status,
			&job.NotifyEnabled,
			&job.WorkerID,
			&job.RootDomainCount,
			&job.SubdomainCount,
			&job.LiveURLCount,
			&job.ErrorMessage,
			&job.StartedAt,
			&job.FinishedAt,
			&job.CreatedAt,
			&job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) ExportSubdomains(ctx context.Context, source string) ([]string, error) {
	query := `SELECT subdomain FROM recon_subdomains`
	args := []any{}
	if strings.TrimSpace(source) != "" {
		query += ` WHERE source = $1`
		args = append(args, strings.TrimSpace(source))
	}
	query += ` ORDER BY subdomain`
	return s.exportSingleColumn(ctx, query, args...)
}

func (s *Store) ExportURLs(ctx context.Context, source string) ([]string, error) {
	query := `SELECT url FROM recon_web_endpoints`
	args := []any{}
	if strings.TrimSpace(source) != "" {
		query += ` WHERE source = $1`
		args = append(args, strings.TrimSpace(source))
	}
	query += ` ORDER BY url`
	return s.exportSingleColumn(ctx, query, args...)
}

func (s *Store) exportSingleColumn(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s *Store) String() string {
	return fmt.Sprintf("store<db=%p>", s.db)
}
