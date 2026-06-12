package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hermes-opencode-team/orchestrator/internal/codeengine"
	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/memory"
	"hermes-opencode-team/orchestrator/internal/workflow"
)

type WorkflowEngine interface {
	RunWorkflow(ctx context.Context, task, sessionID string, useCodeEngine bool) (workflow.RunResult, error)
	ApproveAgentTask(ctx context.Context, sessionID, agentName, task, engine string) (codeengine.Result, error)
}

type MemoryStore interface {
	Recall(ctx context.Context, sessionID string, limit int) ([]memory.Event, error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Bot struct {
	cfg    config.Config
	engine WorkflowEngine
	store  MemoryStore
	client HTTPClient
}

type updateResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type Update struct {
	UpdateID int64   `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	ID int64 `json:"id"`
}

func NewBot(cfg config.Config, engine *workflow.Engine, store *memory.Store) *Bot {
	return &Bot{cfg: cfg, engine: engine, store: store, client: &http.Client{Timeout: 120 * time.Second}}
}

func (b *Bot) Run(ctx context.Context) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			slog.Error("telegram getUpdates", "error", err)
			sleep(ctx, 3*time.Second)
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message.Text == "" {
				continue
			}
			go b.handle(context.Background(), update.Message)
		}
	}
}

func (b *Bot) handle(ctx context.Context, msg Message) {
	if !b.allowed(msg.From.ID) {
		_ = b.send(ctx, msg.Chat.ID, "Access denied")
		return
	}
	text := strings.TrimSpace(msg.Text)
	switch {
	case strings.HasPrefix(text, "/start"):
		_ = b.send(ctx, msg.Chat.ID, "Hermes/OpenCode team ready. Use /task <text>, /approve <session_id> <agent> <task>, /memory <session_id>.")
	case strings.HasPrefix(text, "/task"):
		task := strings.TrimSpace(strings.TrimPrefix(text, "/task"))
		if task == "" {
			_ = b.send(ctx, msg.Chat.ID, "Usage: /task implement health endpoint")
			return
		}
		_ = b.send(ctx, msg.Chat.ID, "Task accepted. Running agents...")
		result, err := b.engine.RunWorkflow(ctx, task, "", true)
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Workflow failed: "+err.Error())
			return
		}
		_ = b.send(ctx, msg.Chat.ID, limit(fmt.Sprintf("session_id: %s\n\n%s", result.SessionID, result.Summary), 3500))
	case strings.HasPrefix(text, "/approve"):
		rest := strings.TrimSpace(strings.TrimPrefix(text, "/approve"))
		parts := strings.SplitN(rest, " ", 3)
		if len(parts) < 3 {
			_ = b.send(ctx, msg.Chat.ID, "Usage: /approve <session_id> <agent> <task>")
			return
		}
		_ = b.send(ctx, msg.Chat.ID, "Approval accepted. Running code engine...")
		result, err := b.engine.ApproveAgentTask(ctx, parts[0], parts[1], parts[2], "")
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Code engine failed: "+err.Error())
			return
		}
		output := result.Stdout
		if output == "" {
			output = result.Stderr
		}
		if output == "" {
			output = fmt.Sprintf("%+v", result)
		}
		_ = b.send(ctx, msg.Chat.ID, limit(output, 3500))
	case strings.HasPrefix(text, "/memory"):
		sessionID := strings.TrimSpace(strings.TrimPrefix(text, "/memory"))
		events, err := b.store.Recall(ctx, sessionID, 10)
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Memory failed: "+err.Error())
			return
		}
		lines := []string{}
		for _, event := range events {
			lines = append(lines, fmt.Sprintf("%s: %s", event.Role, limit(event.Content, 300)))
		}
		if len(lines) == 0 {
			_ = b.send(ctx, msg.Chat.ID, "No memory")
			return
		}
		_ = b.send(ctx, msg.Chat.ID, limit(strings.Join(lines, "\n"), 3500))
	default:
		_ = b.send(ctx, msg.Chat.ID, "Use /task <text>")
	}
}

func (b *Bot) allowed(userID int64) bool {
	if len(b.cfg.TelegramAllowedUserIDs) == 0 {
		return true
	}
	_, ok := b.cfg.TelegramAllowedUserIDs[userID]
	return ok
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=60&offset=%d", b.cfg.TelegramBotToken, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram status %s", resp.Status)
	}
	var data updateResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Result, nil
}

func (b *Bot) send(ctx context.Context, chatID int64, text string) error {
	payload, err := json.Marshal(map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"text":    text,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.cfg.TelegramBotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send status %s", resp.Status)
	}
	return nil
}

func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func limit(input string, max int) string {
	if len(input) <= max {
		return input
	}
	return input[:max]
}
