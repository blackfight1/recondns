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
	"recondns/internal/model"
	"recondns/internal/normalize"
	"recondns/internal/notify"
	"recondns/internal/runner"
)

type Service struct {
	cfg         config.Config
	notifier    *notify.FeishuNotifier
	subfinder   *runner.SubfinderRunner
	chaos       *runner.ChaosRunner
	assetfinder *runner.AssetfinderRunner
	findomain   *runner.FindomainRunner
	rapiddns    *runner.RapidDNSRunner
}

type CollectResult struct {
	Roots      []string
	Subdomains []string
}

func NewService(cfg config.Config) *Service {
	return &Service{
		cfg:         cfg,
		notifier:    notify.NewFeishuNotifier(cfg.FeishuWebhook, true),
		subfinder:   &runner.SubfinderRunner{},
		chaos:       &runner.ChaosRunner{},
		assetfinder: &runner.AssetfinderRunner{},
		findomain:   &runner.FindomainRunner{},
		rapiddns:    &runner.RapidDNSRunner{},
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

func (s *Service) collectSubdomains(ctx context.Context, roots []string) ([]model.SubdomainAsset, error) {
	type result struct {
		tool  string
		hosts []string
		err   error
		dur   time.Duration
	}
	ch := make(chan result, 5)
	var wg sync.WaitGroup

	runCollector := func(tool string, fn func(context.Context, []string) ([]string, error)) {
		defer wg.Done()
		toolStart := time.Now()
		log.Printf("[subs:%s] start roots=%d", tool, len(roots))
		hosts, err := fn(ctx, roots)
		ch <- result{tool: tool, hosts: hosts, err: err, dur: time.Since(toolStart)}
	}

	wg.Add(5)
	go runCollector(s.subfinder.Name(), s.subfinder.Collect)
	go runCollector(s.chaos.Name(), s.chaos.Collect)
	go runCollector(s.assetfinder.Name(), s.assetfinder.Collect)
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

func (s *Service) NotifyText(message string) error {
	if s.notifier == nil || !s.notifier.Enabled() {
		return nil
	}
	return s.notifier.SendText(message)
}
