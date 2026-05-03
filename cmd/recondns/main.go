package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"recondns/internal/app"
	"recondns/internal/config"
	"recondns/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	store, err := storage.New(cfg.DBDSN)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer store.Close()

	service := app.NewService(store, cfg)

	switch os.Args[1] {
	case "submit":
		if err := runSubmit(service, os.Args[2:]); err != nil {
			log.Fatalf("submit failed: %v", err)
		}
	case "run":
		if err := runNow(service, os.Args[2:]); err != nil {
			log.Fatalf("run failed: %v", err)
		}
	case "worker":
		if err := runWorker(service, cfg, os.Args[2:]); err != nil {
			log.Fatalf("worker failed: %v", err)
		}
	case "jobs":
		if err := runJobs(service, os.Args[2:]); err != nil {
			log.Fatalf("jobs failed: %v", err)
		}
	case "export":
		if err := runExport(service, os.Args[2:]); err != nil {
			log.Fatalf("export failed: %v", err)
		}
	case "help", "-h", "--help":
		printUsage()
	default:
		printUsage()
		log.Fatalf("unknown command: %s", os.Args[1])
	}
}

func runSubmit(service *app.Service, args []string) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	inputFile := fs.String("input", "", "Path to root domains file")
	source := fs.String("source", "default", "Logical source label, e.g. h1/bbp")
	notify := fs.Bool("notify", true, "Enable Feishu notifications for this job")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*inputFile) == "" {
		return errors.New("--input is required")
	}

	job, err := service.SubmitJob(context.Background(), *inputFile, *source, *notify)
	if err != nil {
		return err
	}

	fmt.Printf("job submitted: id=%d source=%s roots=%d status=%s\n", job.ID, job.Source, job.RootDomainCount, job.Status)
	return nil
}

func runNow(service *app.Service, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	inputFile := fs.String("input", "", "Path to root domains file")
	source := fs.String("source", "default", "Logical source label, e.g. h1/bbp")
	notify := fs.Bool("notify", true, "Enable Feishu notifications for this job")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*inputFile) == "" {
		return errors.New("--input is required")
	}

	job, err := service.SubmitJob(context.Background(), *inputFile, *source, *notify)
	if err != nil {
		return err
	}
	if err := service.ProcessJobByID(context.Background(), job.ID, cfgWorkerID("run")); err != nil {
		return err
	}
	fmt.Printf("job completed: id=%d\n", job.ID)
	return nil
}

func runWorker(service *app.Service, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	workerID := fs.String("worker-id", cfgWorkerID("worker"), "Worker identifier")
	pollInterval := fs.Duration("poll-interval", cfg.WorkerPollInterval, "Polling interval, e.g. 10s")
	runOnce := fs.Bool("once", false, "Process at most one queued job and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *runOnce {
		processed, err := service.ProcessNextQueuedJob(context.Background(), *workerID)
		if err != nil {
			return err
		}
		if !processed {
			fmt.Println("no queued jobs")
		}
		return nil
	}

	fmt.Printf("worker started: worker_id=%s poll_interval=%s\n", *workerID, pollInterval.String())
	ticker := time.NewTicker(*pollInterval)
	defer ticker.Stop()

	for {
		processed, err := service.ProcessNextQueuedJob(context.Background(), *workerID)
		if err != nil {
			log.Printf("worker iteration failed: %v", err)
		}
		if processed {
			continue
		}
		<-ticker.C
	}
}

func runJobs(service *app.Service, args []string) error {
	fs := flag.NewFlagSet("jobs", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Max rows to print")
	if err := fs.Parse(args); err != nil {
		return err
	}

	jobs, err := service.ListJobs(context.Background(), *limit)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("no jobs")
		return nil
	}

	fmt.Println("ID\tSOURCE\tSTATUS\tROOTS\tSUBS\tURLS\tCREATED")
	for _, job := range jobs {
		fmt.Printf("%d\t%s\t%s\t%d\t%d\t%d\t%s\n",
			job.ID,
			job.Source,
			job.Status,
			job.RootDomainCount,
			job.SubdomainCount,
			job.LiveURLCount,
			job.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}
	return nil
}

func runExport(service *app.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("export requires a target: subdomains or urls")
	}

	target := strings.ToLower(strings.TrimSpace(args[0]))
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	source := fs.String("source", "", "Optional source filter")
	output := fs.String("output", "", "Optional output file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	var lines []string
	var err error
	switch target {
	case "subdomains":
		lines, err = service.ExportSubdomains(context.Background(), *source)
	case "urls":
		lines, err = service.ExportURLs(context.Background(), *source)
	default:
		return fmt.Errorf("unknown export target: %s", target)
	}
	if err != nil {
		return err
	}

	if strings.TrimSpace(*output) != "" {
		if err := os.WriteFile(*output, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %d lines to %s\n", len(lines), *output)
		return nil
	}

	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func cfgWorkerID(prefix string) string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%s", prefix, host)
}

func printUsage() {
	fmt.Println("recondns - subdomain collection backend")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  submit --input h1.txt --source h1")
	fmt.Println("  run --input h1.txt --source h1")
	fmt.Println("  worker --worker-id recon-worker-1")
	fmt.Println("  jobs --limit 20")
	fmt.Println("  export subdomains --source h1 --output subs.txt")
	fmt.Println("  export urls --source h1 --output urls.txt")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  RECONDNS_DB_DSN     PostgreSQL DSN")
	fmt.Println("  FEISHU_WEBHOOK      Feishu bot webhook")
	fmt.Println("  BBOT_PASSIVE_ONLY   true/false (default true)")
}
