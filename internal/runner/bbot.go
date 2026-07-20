package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"recondns/internal/input"
	"recondns/internal/normalize"
)

type BbotRunner struct{}

const bbotScanName = "recondns"

func (r *BbotRunner) Name() string {
	return "bbot"
}

func (r *BbotRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("bbot"); err != nil {
		return nil, fmt.Errorf("bbot not found in PATH")
	}

	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	outDir, err := os.MkdirTemp("", "recondns_bbot_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(outDir)

	targetsPath, err := writeTempTargets(roots)
	if err != nil {
		return nil, err
	}
	defer os.Remove(targetsPath)

	// Passive-only subdomain enum; subdomains.txt is written by the preset's output module.
	args := []string{
		"-t", targetsPath,
		"-p", "subdomain-enum",
		"-rf", "passive",
		"-y",
		"-n", bbotScanName,
		"-o", outDir,
		"-eom", "csv", "txt", "stdout", "json",
	}

	cmd := exec.Command("bbot", args...)
	output, runErr := runCommandWithContext(ctx, cmd)

	subdomainsPath := filepath.Join(outDir, bbotScanName, "subdomains.txt")
	subdomains, parseErr := readOptionalLines(subdomainsPath)
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("bbot execution failed: %v", formatCmdError(runErr, output))
		}
		return nil, parseErr
	}

	subdomains = normalize.Domains(subdomains)
	if runErr != nil && len(subdomains) == 0 {
		return nil, fmt.Errorf("bbot execution failed: %v", formatCmdError(runErr, output))
	}
	if runErr != nil {
		fmt.Printf("[bbot] finished with warning: %v\n", formatCmdError(runErr, output))
	}

	return subdomains, nil
}

func writeTempTargets(roots []string) (string, error) {
	f, err := os.CreateTemp("", "recondns_bbot_targets_*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	for _, root := range roots {
		if _, err := f.WriteString(root + "\n"); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func readOptionalLines(path string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return input.ReadLines(path)
}

func formatCmdError(err error, output []byte) error {
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	// Keep error messages readable; bbot logs can be large.
	if len(msg) > 2000 {
		msg = msg[len(msg)-2000:]
	}
	return fmt.Errorf("%v: %s", err, msg)
}
