package storage

import "context"

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`
CREATE TABLE IF NOT EXISTS recon_jobs (
	id BIGSERIAL PRIMARY KEY,
	source TEXT NOT NULL,
	input_file TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	notify_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	worker_id TEXT NOT NULL DEFAULT '',
	root_domain_count INTEGER NOT NULL DEFAULT 0,
	subdomain_count INTEGER NOT NULL DEFAULT 0,
	error_message TEXT NOT NULL DEFAULT '',
	started_at TIMESTAMPTZ NULL,
	finished_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
`,
		`
CREATE TABLE IF NOT EXISTS recon_root_domains (
	id BIGSERIAL PRIMARY KEY,
	job_id BIGINT NOT NULL REFERENCES recon_jobs(id) ON DELETE CASCADE,
	source TEXT NOT NULL,
	root_domain TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(job_id, root_domain)
)
`,
		`
CREATE TABLE IF NOT EXISTS recon_subdomains (
	id BIGSERIAL PRIMARY KEY,
	root_domain TEXT NOT NULL,
	subdomain TEXT NOT NULL,
	source TEXT NOT NULL,
	discovered_by TEXT NOT NULL DEFAULT '',
	first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	first_job_id BIGINT NOT NULL REFERENCES recon_jobs(id) ON DELETE RESTRICT,
	last_job_id BIGINT NOT NULL REFERENCES recon_jobs(id) ON DELETE RESTRICT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(root_domain, subdomain)
)
`,
		`CREATE INDEX IF NOT EXISTS idx_recon_jobs_status ON recon_jobs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_recon_subdomains_source ON recon_subdomains(source)`,
		`CREATE INDEX IF NOT EXISTS idx_recon_subdomains_root_domain ON recon_subdomains(root_domain)`,
		`ALTER TABLE recon_jobs DROP COLUMN IF EXISTS live_url_count`,
		`DROP TABLE IF EXISTS recon_web_endpoints`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
