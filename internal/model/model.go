package model

import "time"

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type ReconJob struct {
	ID              int64
	Source          string
	InputFile       string
	Status          JobStatus
	NotifyEnabled   bool
	WorkerID        string
	RootDomainCount int
	SubdomainCount  int
	ErrorMessage    string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ReconJobWithRoots struct {
	ReconJob
	RootDomains []string
}

type SubdomainAsset struct {
	RootDomain   string
	Subdomain    string
	DiscoveredBy []string
}
