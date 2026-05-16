package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"recondns/internal/input"
	"recondns/internal/normalize"
)

type FindomainRunner struct{}

func (r *FindomainRunner) Name() string {
	return "findomain"
}

func (r *FindomainRunner) Collect(ctx context.Context, roots []string) ([]string, error) {
	if _, err := exec.LookPath("findomain"); err != nil {
		return nil, fmt.Errorf("findomain not found in PATH")
	}

	roots = normalize.Domains(roots)
	if len(roots) == 0 {
		return nil, nil
	}

	outputFile, err := os.CreateTemp("", "recondns_findomain_out_*.txt")
	if err != nil {
		return nil, err
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		_ = os.Remove(outputPath)
		return nil, err
	}
	defer os.Remove(outputPath)

	var runErr error
	if len(roots) == 1 {
		runErr = r.executeSingle(ctx, roots[0], outputPath)
	} else {
		runErr = r.executeBatch(ctx, roots, outputPath)
	}

	subdomains, parseErr := input.ReadLines(outputPath)
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("findomain execution failed: %v", runErr)
		}
		return nil, parseErr
	}

	subdomains = normalize.Domains(subdomains)
	if runErr != nil && len(subdomains) == 0 {
		return nil, fmt.Errorf("findomain execution failed: %v", runErr)
	}
	if runErr != nil {
		fmt.Printf("[findomain] finished with warning: %v\n", runErr)
	}

	return subdomains, nil
}

func (r *FindomainRunner) executeSingle(ctx context.Context, root, outputPath string) error {
	fmt.Printf("[findomain] single target mode: %s\n", root)
	args := []string{"-t", root, "-q", "-u", outputPath}
	cmd := exec.CommandContext(ctx, "findomain", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return err
}

func (r *FindomainRunner) executeBatch(ctx context.Context, roots []string, outputPath string) error {
	fmt.Printf("[findomain] batch mode with file input: %d roots\n", len(roots))
	inputFile, err := os.CreateTemp("", "recondns_findomain_in_*.txt")
	if err != nil {
		return err
	}
	inputPath := inputFile.Name()
	for _, root := range roots {
		if _, err := inputFile.WriteString(root + "\n"); err != nil {
			_ = inputFile.Close()
			_ = os.Remove(inputPath)
			return err
		}
	}
	if err := inputFile.Close(); err != nil {
		_ = os.Remove(inputPath)
		return err
	}
	defer os.Remove(inputPath)

	args := []string{"-f", inputPath, "-q", "-u", outputPath}
	cmd := exec.CommandContext(ctx, "findomain", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return err
}
