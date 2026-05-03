package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"recondns/internal/normalize"
)

type BBOTRunner struct {
	PassiveOnly bool
}

func (r *BBOTRunner) Name() string {
	return "bbot"
}

func (r *BBOTRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("bbot"); err != nil {
		return nil, fmt.Errorf("bbot not found in PATH")
	}
	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	scanDir, err := os.MkdirTemp("", "recondns_bbot_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scanDir)

	outputFile := filepath.Join(scanDir, "subdomains.txt")
	scanName := fmt.Sprintf("recondns_subs_%d", time.Now().UnixNano())

	args := []string{"-t"}
	args = append(args, roots...)
	args = append(args,
		"-p", "subdomain-enum",
		"-om", "subdomains",
		"-n", scanName,
		"-o", scanDir,
		"-c", fmt.Sprintf("modules.subdomains.output_file=%s", outputFile),
	)
	if r.PassiveOnly {
		args = append(args, "-rf", "passive", "-ef", "aggressive")
	}

	runCtx := ctx
	cancel := func() {}
	if timeout := resolveBBOTTimeout(r.PassiveOnly); timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := exec.Command("bbot", args...)
	_, runErr := runCommandWithContext(runCtx, cmd)
	subdomains, parseErr := readSubdomainsFile(outputFile)
	if parseErr != nil {
		if runErr != nil {
			return nil, runErr
		}
		return nil, parseErr
	}

	if errors.Is(runErr, context.DeadlineExceeded) {
		return subdomains, nil
	}
	if errors.Is(runErr, context.Canceled) && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return subdomains, runErr
}

func resolveBBOTTimeout(passiveOnly bool) time.Duration {
	defaultMin := 25
	key := "BBOT_PASSIVE_TIMEOUT_MIN"
	if !passiveOnly {
		defaultMin = 60
		key = "BBOT_ACTIVE_TIMEOUT_MIN"
	}
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return time.Duration(v) * time.Minute
		}
	}
	if raw := strings.TrimSpace(os.Getenv("BBOT_TIMEOUT_MIN")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return time.Duration(v) * time.Minute
		}
	}
	return time.Duration(defaultMin) * time.Minute
}

func readSubdomainsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]bool)
	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		host := normalize.Domain(scanner.Text())
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	return out, scanner.Err()
}
