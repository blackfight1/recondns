package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type FeishuNotifier struct {
	enabled bool
	webhook string
	client  *http.Client
}

func NewFeishuNotifier(webhook string, enabled bool) *FeishuNotifier {
	webhook = strings.TrimSpace(webhook)
	return &FeishuNotifier{
		enabled: enabled && webhook != "",
		webhook: webhook,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *FeishuNotifier) Enabled() bool {
	return n != nil && n.enabled
}

func (n *FeishuNotifier) SendText(content string) error {
	if !n.Enabled() {
		return nil
	}
	return n.sendText(strings.TrimSpace(content))
}

func (n *FeishuNotifier) sendText(content string) error {
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": content,
		},
	}
	return n.doSend(payload)
}

func (n *FeishuNotifier) doSend(payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, n.webhook, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu notify failed with status: %s", resp.Status)
	}
	return checkFeishuResponse(resp.Body)
}

func checkFeishuResponse(body io.Reader) error {
	var result struct {
		Code          int    `json:"code"`
		Msg           string `json:"msg"`
		StatusCode    int    `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	}
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return nil
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu notify failed: code=%d msg=%s", result.Code, result.Msg)
	}
	if result.StatusCode != 0 {
		return fmt.Errorf("feishu notify failed: status_code=%d status_message=%s", result.StatusCode, result.StatusMessage)
	}
	return nil
}
