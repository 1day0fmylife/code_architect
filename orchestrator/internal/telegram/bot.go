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
	ApproveApproval(ctx context.Context, approvalID, engine string) (codeengine.Result, error)
}

type MemoryStore interface {
	Ping(ctx context.Context) error
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
	OK          bool     `json:"ok"`
	Description string   `json:"description"`
	Result      []Update `json:"result"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
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

type CallbackQuery struct {
	ID      string  `json:"id"`
	Data    string  `json:"data"`
	Message Message `json:"message"`
	From    User    `json:"from"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

func NewBot(cfg config.Config, engine *workflow.Engine, store *memory.Store) *Bot {
	return &Bot{cfg: cfg, engine: engine, store: store, client: &http.Client{Timeout: 120 * time.Second}}
}

func (b *Bot) Run(ctx context.Context) {
	if err := b.deleteWebhook(ctx); err != nil {
		slog.Warn("telegram deleteWebhook", "error", err)
	}

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

			if update.CallbackQuery != nil {
				go b.handleCallback(context.Background(), *update.CallbackQuery)
				continue
			}
			if update.Message == nil || update.Message.Text == "" {
				continue
			}
			go b.handle(context.Background(), *update.Message)
		}
	}
}

func (b *Bot) handle(ctx context.Context, msg Message) {
	if !b.allowed(msg.From.ID) {
		_ = b.send(ctx, msg.Chat.ID, "Access denied", nil)
		return
	}
	text := strings.TrimSpace(msg.Text)
	switch {
	case strings.HasPrefix(text, "/start"), strings.HasPrefix(text, "/menu"):
		_ = b.sendMenu(ctx, msg.Chat.ID)
	case strings.HasPrefix(text, "/help"):
		_ = b.sendHelp(ctx, msg.Chat.ID)
	case strings.HasPrefix(text, "/status"):
		_ = b.sendStatus(ctx, msg.Chat.ID)
	case strings.HasPrefix(text, "/task"):
		task := strings.TrimSpace(strings.TrimPrefix(text, "/task"))
		if task == "" {
			_ = b.send(ctx, msg.Chat.ID, "Usage: /task implement health endpoint", menuKeyboard())
			return
		}
		_ = b.send(ctx, msg.Chat.ID, "Task accepted. Running agents...", nil)
		result, err := b.engine.RunWorkflow(ctx, task, "", true)
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Workflow failed: "+err.Error(), menuKeyboard())
			return
		}
		_ = b.send(ctx, msg.Chat.ID, limit(fmt.Sprintf("session_id: %s\n\n%s", result.SessionID, result.Summary), 3500), menuKeyboard())
	case strings.HasPrefix(text, "/approve"):
		rest := strings.TrimSpace(strings.TrimPrefix(text, "/approve"))
		parts := strings.Fields(rest)
		if len(parts) < 1 || len(parts) > 2 {
			_ = b.send(ctx, msg.Chat.ID, "Usage: /approve <approval_id> [opencode|codex]", menuKeyboard())
			return
		}
		_ = b.send(ctx, msg.Chat.ID, "Approval accepted. Running code engine...", nil)
		engine := ""
		if len(parts) == 2 {
			engine = parts[1]
		}
		result, err := b.engine.ApproveApproval(ctx, parts[0], engine)
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Code engine failed: "+err.Error(), menuKeyboard())
			return
		}
		output := result.Stdout
		if output == "" {
			output = result.Stderr
		}
		if output == "" {
			output = fmt.Sprintf("%+v", result)
		}
		if len(result.ChangedFiles) > 0 {
			output += "\n\nChanged files:\n" + strings.Join(result.ChangedFiles, "\n")
		}
		if result.DiffStat != "" {
			output += "\n\nDiff stat:\n" + result.DiffStat
		}
		_ = b.send(ctx, msg.Chat.ID, limit(output, 3500), menuKeyboard())
	case strings.HasPrefix(text, "/memory"):
		sessionID := strings.TrimSpace(strings.TrimPrefix(text, "/memory"))
		if sessionID == "" {
			_ = b.send(ctx, msg.Chat.ID, "Usage: /memory <session_id>", menuKeyboard())
			return
		}
		events, err := b.store.Recall(ctx, sessionID, 10)
		if err != nil {
			_ = b.send(ctx, msg.Chat.ID, "Memory failed: "+err.Error(), menuKeyboard())
			return
		}
		lines := []string{}
		for _, event := range events {
			lines = append(lines, fmt.Sprintf("%s: %s", event.Role, limit(event.Content, 300)))
		}
		if len(lines) == 0 {
			_ = b.send(ctx, msg.Chat.ID, "No memory", menuKeyboard())
			return
		}
		_ = b.send(ctx, msg.Chat.ID, limit(strings.Join(lines, "\n"), 3500), menuKeyboard())
	default:
		_ = b.send(ctx, msg.Chat.ID, "Use /menu or /task <text>", menuKeyboard())
	}
}

func (b *Bot) handleCallback(ctx context.Context, query CallbackQuery) {
	_ = b.answerCallback(ctx, query.ID)
	if !b.allowed(query.From.ID) {
		_ = b.send(ctx, query.Message.Chat.ID, "Access denied", nil)
		return
	}

	switch query.Data {
	case "menu:main":
		_ = b.sendMenu(ctx, query.Message.Chat.ID)
	case "help":
		_ = b.sendHelp(ctx, query.Message.Chat.ID)
	case "task:prompt":
		_ = b.send(ctx, query.Message.Chat.ID, "Send a task as:\n/task <description>", menuKeyboard())
	case "approve:prompt":
		_ = b.send(ctx, query.Message.Chat.ID, "Run approved code changes as:\n/approve <approval_id> [opencode|codex]", menuKeyboard())
	case "memory:prompt":
		_ = b.send(ctx, query.Message.Chat.ID, "Read session memory as:\n/memory <session_id>", menuKeyboard())
	case "status:prompt":
		_ = b.sendStatus(ctx, query.Message.Chat.ID)
	default:
		_ = b.send(ctx, query.Message.Chat.ID, "Unknown menu action. Use /menu.", menuKeyboard())
	}
}

func (b *Bot) sendMenu(ctx context.Context, chatID int64) error {
	return b.send(ctx, chatID, "Hermes/OpenCode team ready. Choose an action or send a command.", menuKeyboard())
}

func (b *Bot) sendHelp(ctx context.Context, chatID int64) error {
	return b.send(ctx, chatID, strings.Join([]string{
		"Supported commands:",
		"/start - open main menu",
		"/menu - open main menu",
		"/help - show commands",
		"/status - show bot and database status",
		"/task <description> - run agent workflow",
		"/approve <approval_id> [opencode|codex] - run an approved code engine task",
		"/memory <session_id> - show recent session memory",
	}, "\n"), menuKeyboard())
}

func (b *Bot) sendStatus(ctx context.Context, chatID int64) error {
	databaseStatus := "ok"
	if err := b.store.Ping(ctx); err != nil {
		databaseStatus = err.Error()
	}
	return b.send(ctx, chatID, fmt.Sprintf("Hermes Brain: running\nDatabase: %s", databaseStatus), menuKeyboard())
}

func menuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		{
			{Text: "New task", CallbackData: "task:prompt"},
			{Text: "Approve", CallbackData: "approve:prompt"},
		},
		{
			{Text: "Memory", CallbackData: "memory:prompt"},
			{Text: "Status", CallbackData: "status:prompt"},
		},
		{
			{Text: "Help", CallbackData: "help"},
			{Text: "Menu", CallbackData: "menu:main"},
		},
	}}
}

func (b *Bot) allowed(userID int64) bool {
	if len(b.cfg.TelegramAllowedUserIDs) == 0 {
		return true
	}
	_, ok := b.cfg.TelegramAllowedUserIDs[userID]
	return ok
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	url := b.telegramURL(fmt.Sprintf("getUpdates?timeout=60&offset=%d", offset))
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
	if !data.OK {
		if data.Description == "" {
			data.Description = "telegram api returned ok=false"
		}
		return nil, fmt.Errorf("%s", data.Description)
	}
	return data.Result, nil
}

func (b *Bot) deleteWebhook(ctx context.Context) error {
	payload, err := json.Marshal(map[string]bool{"drop_pending_updates": false})
	if err != nil {
		return err
	}
	return b.postTelegram(ctx, "deleteWebhook", payload)
}

func (b *Bot) answerCallback(ctx context.Context, callbackID string) error {
	if callbackID == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"callback_query_id": callbackID})
	if err != nil {
		return err
	}
	return b.postTelegram(ctx, "answerCallbackQuery", payload)
}

func (b *Bot) send(ctx context.Context, chatID int64, text string, keyboard *InlineKeyboardMarkup) error {
	request := map[string]any{
		"chat_id": strconv.FormatInt(chatID, 10),
		"text":    text,
	}
	if keyboard != nil {
		request["reply_markup"] = keyboard
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return b.postTelegram(ctx, "sendMessage", payload)
}

func (b *Bot) postTelegram(ctx context.Context, method string, payload []byte) error {
	url := b.telegramURL(method)
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

func (b *Bot) telegramURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.cfg.TelegramBotToken, method)
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
