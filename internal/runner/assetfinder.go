package runner

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"recondns/internal/normalize"
)

type AssetfinderRunner struct{}

func (r *AssetfinderRunner) Name() string {
	return "assetfinder"
}

func (r *AssetfinderRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("assetfinder"); err != nil {
		return nil, fmt.Errorf("assetfinder not found in PATH")
	}

	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, 256)
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

func (r *AssetfinderRunner) collectForRoot(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "assetfinder", "--subs-only", root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, 128)
	out := make([]string, 0, 128)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		host := normalize.Domain(scanner.Text())
		if host == "" || !strings.Contains(host, ".") || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	if err := cmd.Wait(); err != nil {
		return out, err
	}
	return out, nil
}
