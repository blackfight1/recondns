package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"recondns/internal/normalize"
)

type ChaosRunner struct{}

const defaultChaosAPIKey = "2d08ed41-599e-4b1d-a13c-6a35282b8e60"

func (r *ChaosRunner) Name() string {
	return "chaos"
}

func (r *ChaosRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("chaos"); err != nil {
		return nil, fmt.Errorf("chaos not found in PATH")
	}
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	args := []string{"-silent"}
	if len(roots) == 1 {
		args = append(args, "-d", roots[0])
	} else {
		tmpFile, err := os.CreateTemp("", "recondns_chaos_*.txt")
		if err != nil {
			return nil, err
		}
		defer os.Remove(tmpFile.Name())
		for _, root := range roots {
			if _, err := tmpFile.WriteString(root + "\n"); err != nil {
				_ = tmpFile.Close()
				return nil, err
			}
		}
		_ = tmpFile.Close()
		args = append(args, "-dL", tmpFile.Name())
	}

	if key := resolveChaosAPIKey(); key != "" {
		args = append(args, "-key", key)
	}

	cmd := exec.CommandContext(ctx, "chaos", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	out := make([]string, 0, 256)
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
		if len(out) == 0 {
			return nil, nil
		}
		return out, err
	}
	return out, nil
}

func resolveChaosAPIKey() string {
	for _, key := range []string{
		strings.TrimSpace(os.Getenv("CHAOS_KEY")),
		strings.TrimSpace(os.Getenv("PDCP_API_KEY")),
	} {
		if key != "" {
			return key
		}
	}
	return defaultChaosAPIKey
}
