package config

import (
	"os"
	"strings"
	"time"
)

const defaultFeishuWebhook = "https://open.feishu.cn/open-apis/bot/v2/hook/0ef53dc3-91cf-43f2-abd5-8dc4d94c7b63"

type Config struct {
	DBDSN              string
	WorkerPollInterval time.Duration
	BBOTPassiveOnly    bool
	FeishuWebhook      string
}

func Load() (Config, error) {
	cfg := Config{
		DBDSN:              strings.TrimSpace(os.Getenv("RECONDNS_DB_DSN")),
		WorkerPollInterval: 10 * time.Second,
		BBOTPassiveOnly:    envBool("BBOT_PASSIVE_ONLY", true),
		FeishuWebhook:      firstNonEmpty(strings.TrimSpace(os.Getenv("FEISHU_WEBHOOK")), defaultFeishuWebhook),
	}

	if raw := strings.TrimSpace(os.Getenv("RECONDNS_WORKER_POLL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			cfg.WorkerPollInterval = d
		}
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envBool(key string, defaultVal bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}
