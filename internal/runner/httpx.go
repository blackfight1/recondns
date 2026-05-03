package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"recondns/internal/model"
	"recondns/internal/normalize"
)

type HTTPXRunner struct {
	TimeoutSec int
	Retries    int
}

type httpxResult struct {
	URL         string   `json:"url"`
	StatusCode  int      `json:"status_code"`
	Title       string   `json:"title"`
	Tech        []string `json:"tech"`
	Host        string   `json:"host"`
	HostIP      string   `json:"host_ip"`
	A           []string `json:"a"`
	ContentType string   `json:"content_type"`
	Webserver   string   `json:"webserver"`
	CDN         bool     `json:"cdn"`
	CDNName     string   `json:"cdn_name"`
	Input       string   `json:"input"`
}

func (r *HTTPXRunner) Name() string {
	return "httpx"
}

func (r *HTTPXRunner) Probe(ctx context.Context, roots []string, subdomains []string) ([]model.WebEndpoint, error) {
	if _, err := exec.LookPath("httpx"); err != nil {
		return nil, fmt.Errorf("httpx not found in PATH")
	}
	subdomains = normalize.Domains(subdomains)
	if len(subdomains) == 0 {
		return nil, nil
	}

	tmpFile, err := os.CreateTemp("", "recondns_httpx_*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	for _, host := range subdomains {
		if _, err := tmpFile.WriteString(host + "\n"); err != nil {
			_ = tmpFile.Close()
			return nil, err
		}
	}
	_ = tmpFile.Close()

	timeout := r.TimeoutSec
	if timeout <= 0 {
		timeout = 10
	}
	retries := r.Retries
	if retries < 0 {
		retries = 0
	}

	cmd := exec.CommandContext(ctx, "httpx",
		"-l", tmpFile.Name(),
		"-json",
		"-sc",
		"-title",
		"-td",
		"-ip",
		"-silent",
		"-timeout", strconv.Itoa(timeout),
		"-retries", strconv.Itoa(retries),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var out []model.WebEndpoint
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item httpxResult
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if strings.TrimSpace(item.URL) == "" || seen[item.URL] {
			continue
		}
		seen[item.URL] = true
		root := normalize.MatchRootDomain(item.Host, roots)
		ip := strings.TrimSpace(item.HostIP)
		if ip == "" && len(item.A) > 0 {
			ip = strings.TrimSpace(item.A[0])
		}
		scheme, port := parseSchemeAndPort(item.URL)
		host := normalize.Domain(item.Host)
		if host == "" {
			host = normalize.Domain(item.Input)
		}
		out = append(out, model.WebEndpoint{
			RootDomain:  root,
			Subdomain:   host,
			URL:         strings.TrimSpace(item.URL),
			Host:        host,
			Scheme:      scheme,
			Port:        port,
			StatusCode:  item.StatusCode,
			Title:       strings.TrimSpace(item.Title),
			Tech:        item.Tech,
			IP:          ip,
			Webserver:   strings.TrimSpace(item.Webserver),
			CDN:         item.CDN,
			CDNName:     strings.TrimSpace(item.CDNName),
			ContentType: strings.TrimSpace(item.ContentType),
		})
	}
	if err := cmd.Wait(); err != nil {
		return out, err
	}
	return out, scanner.Err()
}

func parseSchemeAndPort(rawURL string) (string, int) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", 0
	}
	port := 0
	if rawPort := parsed.Port(); rawPort != "" {
		if v, err := strconv.Atoi(rawPort); err == nil {
			port = v
		}
	} else {
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			port = 443
		case "http":
			port = 80
		}
	}
	return strings.ToLower(parsed.Scheme), port
}
