package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"hermes-opencode-team/orchestrator/internal/codeengine"
	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/memory"
	"hermes-opencode-team/orchestrator/internal/workflow"
)

type fakeEngine struct {
	runTask    string
	approvalID string
	approve    approveCall
	approveOut codeengine.Result
}

type approveCall struct {
	sessionID string
	agent     string
	task      string
	engine    string
}

func (f *fakeEngine) RunWorkflow(_ context.Context, task, sessionID string, useCodeEngine bool) (workflow.RunResult, error) {
	f.runTask = task
	return workflow.RunResult{SessionID: "session-1", Summary: "summary ok"}, nil
}

func (f *fakeEngine) ApproveAgentTask(_ context.Context, sessionID, agentName, task, engine string) (codeengine.Result, error) {
	f.approve = approveCall{sessionID: sessionID, agent: agentName, task: task, engine: engine}
	if f.approveOut.Status == "" {
		f.approveOut = codeengine.Result{Status: "ok", Stdout: "changed files", ChangedFiles: []string{"README.md"}}
	}
	return f.approveOut, nil
}

func (f *fakeEngine) ApproveApproval(_ context.Context, approvalID, engine string) (codeengine.Result, error) {
	f.approvalID = approvalID
	f.approve.engine = engine
	if f.approveOut.Status == "" {
		f.approveOut = codeengine.Result{Status: "ok", Stdout: "changed files", ChangedFiles: []string{"README.md"}}
	}
	return f.approveOut, nil
}

type fakeStore struct {
	events []memory.Event
}

func (f fakeStore) Ping(context.Context) error {
	return nil
}

func (f fakeStore) Recall(context.Context, string, int) ([]memory.Event, error) {
	return f.events, nil
}

type sentMessage struct {
	ChatID      string                `json:"chat_id"`
	Text        string                `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup"`
}

type fakeHTTPClient struct {
	messages  []sentMessage
	callbacks int
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/sendMessage") {
		var msg sentMessage
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
			return nil, err
		}
		f.messages = append(f.messages, msg)
	}
	if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/answerCallbackQuery") {
		f.callbacks++
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":[]}`)),
	}, nil
}

func TestHandleDeniesUnknownUser(t *testing.T) {
	client := &fakeHTTPClient{}
	engine := &fakeEngine{}
	bot := testBot(engine, fakeStore{}, client, map[int64]struct{}{42: {}})

	bot.handle(context.Background(), testMessage(7, "/task ship it"))

	if engine.runTask != "" {
		t.Fatalf("expected denied user not to run workflow, got task %q", engine.runTask)
	}
	requireLastMessage(t, client, "Access denied")
}

func TestHandleTaskRunsWorkflow(t *testing.T) {
	client := &fakeHTTPClient{}
	engine := &fakeEngine{}
	bot := testBot(engine, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/task build auth"))

	if engine.runTask != "build auth" {
		t.Fatalf("expected task to reach engine, got %q", engine.runTask)
	}
	requireLastMessage(t, client, "session_id: session-1")
}

func TestHandleStartSendsMenuKeyboard(t *testing.T) {
	client := &fakeHTTPClient{}
	bot := testBot(&fakeEngine{}, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/start"))

	requireLastMessage(t, client, "Hermes/OpenCode team ready")
	if client.messages[len(client.messages)-1].ReplyMarkup == nil {
		t.Fatal("expected menu keyboard")
	}
}

func TestHandleCallbackShowsTaskPrompt(t *testing.T) {
	client := &fakeHTTPClient{}
	bot := testBot(&fakeEngine{}, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handleCallback(context.Background(), CallbackQuery{
		ID:      "callback-1",
		Data:    "task:prompt",
		From:    User{ID: 7},
		Message: Message{Chat: Chat{ID: 100}},
	})

	if client.callbacks != 1 {
		t.Fatalf("expected callback answer, got %d", client.callbacks)
	}
	requireLastMessage(t, client, "/task <description>")
}

func TestHandleApproveRunsCodeEngine(t *testing.T) {
	client := &fakeHTTPClient{}
	engine := &fakeEngine{}
	bot := testBot(engine, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/approve appr_123 codex"))

	if engine.approvalID != "appr_123" || engine.approve.engine != "codex" {
		t.Fatalf("unexpected approve call: approvalID=%q call=%#v", engine.approvalID, engine.approve)
	}
	requireLastMessage(t, client, "changed files")
	requireLastMessage(t, client, "README.md")
}

func TestHandleMemoryPrintsEvents(t *testing.T) {
	client := &fakeHTTPClient{}
	store := fakeStore{events: []memory.Event{{
		SessionID: "session-1",
		Role:      "architect",
		Content:   "plan",
		CreatedAt: time.Now(),
	}}}
	bot := testBot(&fakeEngine{}, store, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/memory session-1"))

	requireLastMessage(t, client, "architect: plan")
}

func TestHandleStatusPingsStore(t *testing.T) {
	client := &fakeHTTPClient{}
	bot := testBot(&fakeEngine{}, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/status"))

	requireLastMessage(t, client, "Database: ok")
}

func testBot(engine *fakeEngine, store fakeStore, client *fakeHTTPClient, allowed map[int64]struct{}) *Bot {
	return &Bot{
		cfg:    config.Config{TelegramBotToken: "test-token", TelegramAllowedUserIDs: allowed},
		engine: engine,
		store:  store,
		client: client,
	}
}

func testMessage(userID int64, text string) Message {
	return Message{Text: text, Chat: Chat{ID: 100}, From: User{ID: userID}}
}

func requireLastMessage(t *testing.T, client *fakeHTTPClient, want string) {
	t.Helper()
	if len(client.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	got := client.messages[len(client.messages)-1].Text
	if !strings.Contains(got, want) {
		t.Fatalf("expected last message to contain %q, got %q", want, got)
	}
}
