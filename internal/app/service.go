package app

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"recondns/internal/config"
	"recondns/internal/input"
	"recondns/internal/model"
	"recondns/internal/normalize"
	"recondns/internal/notify"
	"recondns/internal/runner"
	"recondns/internal/storage"
)

type Service struct {
	store     *storage.Store
	cfg       config.Config
	notifier  *notify.FeishuNotifier
	subfinder *runner.SubfinderRunner
	chaos     *runner.ChaosRunner
	findomain *runner.FindomainRunner
	rapiddns  *runner.RapidDNSRunner
}

type CollectResult struct {
	Roots      []string
	Subdomains []string
}

func NewService(store *storage.Store, cfg config.Config) *Service {
	return &Service{
		store:     store,
		cfg:       cfg,
		notifier:  notify.NewFeishuNotifier(cfg.FeishuWebhook, true),
		subfinder: &runner.SubfinderRunner{},
		chaos:     &runner.ChaosRunner{},
		findomain: &runner.FindomainRunner{},
		rapiddns:  &runner.RapidDNSRunner{},
	}
}

func (s *Service) Collect(ctx context.Context, roots []string) (CollectResult, error) {
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return CollectResult{}, fmt.Errorf("no valid root domains provided")
	}

	result := CollectResult{
		Roots: roots,
	}

	subdomains, err := s.collectSubdomains(ctx, roots)
	if err != nil {
		return result, err
	}

	subs := make([]string, 0, len(subdomains))
	for _, item := range subdomains {
		if strings.TrimSpace(item.Subdomain) == "" {
			continue
		}
		subs = append(subs, item.Subdomain)
	}
	sort.Strings(subs)
	result.Subdomains = subs

	return result, nil
}

func (s *Service) SubmitJob(ctx context.Context, inputFile, source string, notifyEnabled bool) (model.ReconJob, error) {
	roots, err := input.ReadLines(inputFile)
	if err != nil {
		return model.ReconJob{}, err
	}
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return model.ReconJob{}, fmt.Errorf("no valid root domains found in %s", inputFile)
	}

	job, err := s.store.SubmitJob(ctx, source, inputFile, roots, notifyEnabled)
	if err != nil {
		return model.ReconJob{}, err
	}
	if notifyEnabled {
		_ = s.notifier.SendJobQueued(job.ID, job.Source, roots)
	}
	return job, nil
}

func (s *Service) ProcessNextQueuedJob(ctx context.Context, workerID string) (bool, error) {
	job, ok, err := s.store.ClaimNextQueuedJob(ctx, workerID)
	if err != nil || !ok {
		return ok, err
	}
	log.Printf("[job:%d] claimed source=%s roots=%d worker=%s", job.ID, job.Source, len(job.RootDomains), workerID)
	return true, s.processJob(ctx, job)
}

func (s *Service) ProcessJobByID(ctx context.Context, jobID int64, workerID string) error {
	job, err := s.store.LoadJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == model.JobQueued {
		claimed, ok, err := s.store.ClaimNextQueuedJob(ctx, workerID)
		if err == nil && ok && claimed.ID == jobID {
			job = claimed
		}
	}
	if job.Status != model.JobRunning {
		now := time.Now().UTC()
		job.Status = model.JobRunning
		job.WorkerID = workerID
		job.StartedAt = &now
	}
	return s.processJob(ctx, job)
}

func (s *Service) processJob(ctx context.Context, job model.ReconJobWithRoots) error {
	start := time.Now()
	log.Printf("[job:%d] start source=%s roots=%d input=%s", job.ID, job.Source, len(job.RootDomains), job.InputFile)
	if job.NotifyEnabled {
		_ = s.notifier.SendJobStart(job.ID, job.Source, job.RootDomains)
	}

	subdomains, err := s.collectSubdomains(ctx, job.RootDomains)
	if err != nil {
		log.Printf("[job:%d] subdomain collection failed after %s: %v", job.ID, time.Since(start).Round(time.Second), err)
		_ = s.store.MarkJobFailed(ctx, job.ID, err.Error())
		if job.NotifyEnabled {
			_ = s.notifier.SendJobEnd(job.ID, job.Source, false, time.Since(start), 0, 0, err.Error())
		}
		return err
	}
	log.Printf("[job:%d] subdomain collection complete unique=%d", job.ID, len(subdomains))

	log.Printf("[job:%d] writing subdomains to database", job.ID)
	if err := s.store.UpsertSubdomains(ctx, job, subdomains); err != nil {
		log.Printf("[job:%d] subdomain DB write failed: %v", job.ID, err)
		_ = s.store.MarkJobFailed(ctx, job.ID, err.Error())
		if job.NotifyEnabled {
			_ = s.notifier.SendJobEnd(job.ID, job.Source, false, time.Since(start), len(subdomains), 0, err.Error())
		}
		return err
	}

	if err := s.store.MarkJobSucceeded(ctx, job.ID, len(subdomains)); err != nil {
		return err
	}
	log.Printf("[job:%d] success duration=%s subdomains=%d", job.ID, time.Since(start).Round(time.Second), len(subdomains))
	if job.NotifyEnabled {
		_ = s.notifier.SendJobEnd(job.ID, job.Source, true, time.Since(start), len(subdomains), 0, "")
	}
	return nil
}

func (s *Service) collectSubdomains(ctx context.Context, roots []string) ([]model.SubdomainAsset, error) {
	type result struct {
		tool  string
		hosts []string
		err   error
		dur   time.Duration
	}
	ch := make(chan result, 4)
	var wg sync.WaitGroup

	runCollector := func(tool string, fn func(context.Context, []string) ([]string, error)) {
		defer wg.Done()
		toolStart := time.Now()
		log.Printf("[subs:%s] start roots=%d", tool, len(roots))
		hosts, err := fn(ctx, roots)
		ch <- result{tool: tool, hosts: hosts, err: err, dur: time.Since(toolStart)}
	}

	wg.Add(4)
	go runCollector(s.subfinder.Name(), s.subfinder.Collect)
	go runCollector(s.chaos.Name(), s.chaos.Collect)
	go runCollector(s.findomain.Name(), s.findomain.Collect)
	go runCollector(s.rapiddns.Name(), s.rapiddns.Collect)

	go func() {
		wg.Wait()
		close(ch)
	}()

	merged := make(map[string]map[string]bool)
	var errs []string
	for item := range ch {
		if item.err != nil {
			log.Printf("[subs:%s] finished with warning duration=%s results=%d err=%v", item.tool, item.dur.Round(time.Second), len(item.hosts), item.err)
		} else {
			log.Printf("[subs:%s] finished duration=%s results=%d", item.tool, item.dur.Round(time.Second), len(item.hosts))
		}
		if item.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", item.tool, item.err))
		}
		for _, host := range item.hosts {
			host = normalize.Domain(host)
			root := normalize.MatchRootDomain(host, roots)
			if root == "" {
				continue
			}
			if merged[host] == nil {
				merged[host] = make(map[string]bool)
			}
			merged[host][item.tool] = true
		}
	}

	if len(merged) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf(strings.Join(errs, " | "))
	}

	out := make([]model.SubdomainAsset, 0, len(merged))
	for host, tools := range merged {
		root := normalize.MatchRootDomain(host, roots)
		discoveredBy := make([]string, 0, len(tools))
		for tool := range tools {
			discoveredBy = append(discoveredBy, tool)
		}
		out = append(out, model.SubdomainAsset{
			RootDomain:   root,
			Subdomain:    host,
			DiscoveredBy: discoveredBy,
		})
	}
	return out, nil
}

func (s *Service) ListJobs(ctx context.Context, limit int) ([]model.ReconJob, error) {
	return s.store.ListJobs(ctx, limit)
}

func (s *Service) ExportSubdomains(ctx context.Context, source string) ([]string, error) {
	return s.store.ExportSubdomains(ctx, source)
}

func (s *Service) ResetAll(ctx context.Context) error {
	return s.store.ResetAll(ctx)
}

func (s *Service) NotifyText(message string) error {
	if s.notifier == nil || !s.notifier.Enabled() {
		return nil
	}
	return s.notifier.SendText(message)
}
