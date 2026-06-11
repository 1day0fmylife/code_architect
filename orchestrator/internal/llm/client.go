package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hermes-opencode-team/orchestrator/internal/config"
)

type Client struct {
	cfg  config.Config
	http *http.Client
}

func NewClient(cfg config.Config) Client {
	return Client{cfg: cfg, http: &http.Client{Timeout: 300 * time.Second}}
}

func (c Client) Chat(ctx context.Context, prompt, model, backend string) (string, error) {
	if backend == "" {
		backend = c.cfg.DefaultLLMBackend
	}
	if model == "" {
		model = c.cfg.DefaultModel
	}

	switch backend {
	case "ollama":
		return c.chatOllama(ctx, prompt, model)
	case "llamacpp":
		return c.chatOpenAICompatible(ctx, prompt, model)
	default:
		return "", fmt.Errorf("unsupported backend: %s", backend)
	}
}

func (c Client) chatOllama(ctx context.Context, prompt, model string) (string, error) {
	payload := map[string]any{
		"model":  model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := c.postJSON(ctx, strings.TrimRight(c.cfg.OllamaBaseURL, "/")+"/api/chat", payload, &response); err != nil {
		return "", err
	}
	return response.Message.Content, nil
}

func (c Client) chatOpenAICompatible(ctx context.Context, prompt, model string) (string, error) {
	payload := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := c.postJSON(ctx, strings.TrimRight(c.cfg.LlamaCPPBaseURL, "/")+"/chat/completions", payload, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("llamacpp returned no choices")
	}
	return response.Choices[0].Message.Content, nil
}

func (c Client) postJSON(ctx context.Context, url string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("llm request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
