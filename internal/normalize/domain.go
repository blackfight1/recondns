package normalize

import (
	"net/url"
	"sort"
	"strings"
)

func Domain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimPrefix(value, "*.")
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Hostname()
		}
	}
	return value
}

func Domains(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		d := Domain(value)
		if d == "" || !strings.Contains(d, ".") || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func MatchRootDomain(host string, roots []string) string {
	host = Domain(host)
	if host == "" {
		return ""
	}
	best := ""
	for _, root := range roots {
		root = Domain(root)
		if root == "" {
			continue
		}
		if host == root || strings.HasSuffix(host, "."+root) {
			if len(root) > len(best) {
				best = root
			}
		}
	}
	return best
}
