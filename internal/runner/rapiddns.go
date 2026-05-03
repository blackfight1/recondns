package runner

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"recondns/internal/normalize"
)

type RapidDNSRunner struct{}

func (r *RapidDNSRunner) Name() string {
	return "rapiddns"
}

func (r *RapidDNSRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("rapiddns-cli"); err != nil {
		return nil, fmt.Errorf("rapiddns-cli not found in PATH")
	}
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	out := make([]string, 0, 256)

	for _, root := range roots {
		hosts, err := r.collectForRoot(ctx, root)
		if err != nil {
			return out, err
		}
		for _, host := range hosts {
			if seen[host] {
				continue
			}
			seen[host] = true
			out = append(out, host)
		}
	}

	return out, nil
}

func (r *RapidDNSRunner) collectForRoot(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "rapiddns-cli",
		"search", root,
		"--column", "subdomain",
		"-o", "text",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var out []string
	parseScanner := func(scanner *bufio.Scanner) {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			// Filter progress chatter such as:
			// "Fetching up to 20 records...", "Fetched 100 records...", "Done."
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "fetching ") || strings.HasPrefix(lower, "fetched ") || lower == "done." {
				continue
			}
			host := normalize.Domain(line)
			if host == "" || !strings.Contains(host, ".") || seen[host] {
				continue
			}
			seen[host] = true
			out = append(out, host)
		}
	}

	parseScanner(bufio.NewScanner(stdout))
	parseScanner(bufio.NewScanner(stderr))

	if err := cmd.Wait(); err != nil {
		return out, err
	}
	return out, nil
}
