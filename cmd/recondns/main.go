package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"recondns/internal/app"
	"recondns/internal/config"
	"recondns/internal/input"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	args := os.Args[1:]
	for _, arg := range args {
		switch arg {
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}

	if err := run(args); err != nil {
		printUsage()
		log.Fatalf("run failed: %v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("recondns", flag.ContinueOnError)
	domain := fs.String("d", "", "Single root domain")
	domainList := fs.String("dL", "", "Input file with one root domain per line")
	output := fs.String("o", "", "Output file")
	jsonOutput := fs.Bool("json", false, "Output JSON")
	notifyEnabled := fs.Bool("notify", false, "Send Feishu notification after run")
	if err := fs.Parse(args); err != nil {
		return err
	}

	roots, err := resolveRoots(*domain, *domainList)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	service := app.NewService(cfg)
	result, err := service.Collect(context.Background(), roots)
	if err != nil {
		return err
	}

	if err := writeOutput(result, *output, *jsonOutput); err != nil {
		return err
	}

	if *notifyEnabled {
		message := fmt.Sprintf(
			"[Recondns] Finished\nRoots: %d\nSubdomains: %d",
			len(result.Roots),
			len(result.Subdomains),
		)
		if err := service.NotifyText(message); err != nil {
			log.Printf("notify warning: %v", err)
		}
	}

	return nil
}

func resolveRoots(domain, domainList string) ([]string, error) {
	hasDomain := strings.TrimSpace(domain) != ""
	hasList := strings.TrimSpace(domainList) != ""

	if hasDomain && hasList {
		return nil, errors.New("use either -d or -dL, not both")
	}
	if hasDomain {
		return []string{strings.TrimSpace(domain)}, nil
	}
	if hasList {
		lines, err := input.ReadLines(strings.TrimSpace(domainList))
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			return nil, fmt.Errorf("no root domains found in %s", domainList)
		}
		return lines, nil
	}

	return nil, errors.New("usage: recondns -d hackerone.com -o h1-subs.txt | recondns -dL h1.txt -o h1-subs.txt")
}

func writeOutput(result app.CollectResult, output string, jsonOutput bool) error {
	var (
		data []byte
		err  error
	)

	if jsonOutput {
		data, err = json.MarshalIndent(struct {
			Roots      []string `json:"roots"`
			Subdomains []string `json:"subdomains"`
		}{
			Roots:      result.Roots,
			Subdomains: result.Subdomains,
		}, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	} else {
		data = []byte(strings.Join(result.Subdomains, "\n"))
		if len(result.Subdomains) > 0 {
			data = append(data, '\n')
		}
	}

	if strings.TrimSpace(output) != "" {
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Printf("wrote JSON with %d subdomains to %s\n", len(result.Subdomains), output)
		} else {
			fmt.Printf("wrote %d lines to %s\n", len(result.Subdomains), output)
		}
		return nil
	}

	_, err = os.Stdout.Write(data)
	return err
}

func printUsage() {
	fmt.Println("recondns - subdomain enum CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  recondns -d hackerone.com -o h1-subs.txt")
	fmt.Println("  recondns -dL h1.txt -o h1-subs.txt")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -d        single root domain")
	fmt.Println("  -dL       input file, one root domain per line")
	fmt.Println("  -o        output file, default stdout")
	fmt.Println("  -json     output JSON with roots and subdomains")
	fmt.Println("  -notify   send Feishu notification")
	fmt.Println()
	fmt.Println("Collectors:")
	fmt.Println("  subfinder")
	fmt.Println("  chaos")
	fmt.Println("  assetfinder")
	fmt.Println("  findomain")
	fmt.Println("  rapiddns-cli")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  CHAOS_KEY           Chaos API key (optional)")
	fmt.Println("  PDCP_API_KEY        Alternate Chaos API key env name")
	fmt.Println("  FEISHU_WEBHOOK      Feishu bot webhook")
}
