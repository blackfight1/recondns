package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"recondns/internal/normalize"
)

type SubfinderRunner struct{}

type subfinderResult struct {
	Host string `json:"host"`
}

func (r *SubfinderRunner) Name() string {
	return "subfinder"
}

func (r *SubfinderRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("subfinder"); err != nil {
		return nil, fmt.Errorf("subfinder not found in PATH")
	}
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	tmpFile, err := os.CreateTemp("", "recondns_subfinder_*.txt")
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

	cmd := exec.CommandContext(ctx, "subfinder", "-dL", tmpFile.Name(), "-all", "-json", "-silent")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var out []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item subfinderResult
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		host := normalize.Domain(item.Host)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	if err := cmd.Wait(); err != nil {
		return out, err
	}
	return out, scanner.Err()
}
