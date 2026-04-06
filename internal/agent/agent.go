package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"tg_chat/internal/config"
	"tg_chat/internal/session"
)

type Client struct {
	httpClient *http.Client
	model      config.ModelConfig
	system     string
}

func LoadSystemPrompt(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func New(model config.ModelConfig, system string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 45 * time.Second},
		model:      model,
		system:     system,
	}
}

type chatCompletionRequest struct {
	Model    string           `json:"model"`
	Messages []map[string]any `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) Reply(ctx context.Context, contextMsgs []session.Message) (string, error) {
	msgs := make([]map[string]any, 0, len(contextMsgs)+1)
	msgs = append(msgs, map[string]any{"role": "system", "content": c.system})
	for _, m := range contextMsgs {
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}
	return c.chat(ctx, msgs)
}

func (c *Client) Summarize(ctx context.Context, summarizeInput string) (string, error) {
	msgs := []map[string]any{
		{"role": "system", "content": "你是会话压缩器。请输出精炼摘要，保留关键约束、事实、用户偏好、待办和未决问题。控制在 500 中文字以内。"},
		{"role": "user", "content": summarizeInput},
	}
	return c.chat(ctx, msgs)
}

func (c *Client) chat(ctx context.Context, msgs []map[string]any) (string, error) {
	reqBody := chatCompletionRequest{Model: c.model.Model, Messages: msgs}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.model.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.model.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("model api status %d: %s", resp.StatusCode, string(respBody))
	}

	var out chatCompletionResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty model response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
