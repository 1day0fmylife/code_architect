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
		f.approveOut = codeengine.Result{Status: "ok", Stdout: "changed files"}
	}
	return f.approveOut, nil
}

type fakeStore struct {
	events []memory.Event
}

func (f fakeStore) Recall(context.Context, string, int) ([]memory.Event, error) {
	return f.events, nil
}

type sentMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type fakeHTTPClient struct {
	messages []sentMessage
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/sendMessage") {
		var msg sentMessage
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil {
			return nil, err
		}
		f.messages = append(f.messages, msg)
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

func TestHandleApproveRunsCodeEngine(t *testing.T) {
	client := &fakeHTTPClient{}
	engine := &fakeEngine{}
	bot := testBot(engine, fakeStore{}, client, map[int64]struct{}{7: {}})

	bot.handle(context.Background(), testMessage(7, "/approve session-1 backend implement endpoint"))

	if engine.approve.sessionID != "session-1" || engine.approve.agent != "backend" || engine.approve.task != "implement endpoint" {
		t.Fatalf("unexpected approve call: %#v", engine.approve)
	}
	requireLastMessage(t, client, "changed files")
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
