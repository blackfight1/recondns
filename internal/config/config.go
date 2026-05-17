package config

import (
	"os"
	"strings"
)

const defaultFeishuWebhook = "https://open.feishu.cn/open-apis/bot/v2/hook/0ef53dc3-91cf-43f2-abd5-8dc4d94c7b63"

type Config struct {
	FeishuWebhook string
}

func Load() (Config, error) {
	cfg := Config{
		FeishuWebhook: firstNonEmpty(strings.TrimSpace(os.Getenv("FEISHU_WEBHOOK")), defaultFeishuWebhook),
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
