package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hermes-opencode-team/orchestrator/internal/codeengine"
	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/memory"
	"hermes-opencode-team/orchestrator/internal/workflow"
)

type fakeStore struct {
	events []memory.Event
	err    error
}

func (f fakeStore) Ping(context.Context) error {
	return f.err
}

func (f fakeStore) Recall(context.Context, string, int) ([]memory.Event, error) {
	return f.events, f.err
}

type fakeEngine struct {
	runTask       string
	runSessionID  string
	runCodeEngine bool
	runErr        error
}

func (f *fakeEngine) RunWorkflow(_ context.Context, task, sessionID string, useCodeEngine bool) (workflow.RunResult, error) {
	f.runTask = task
	f.runSessionID = sessionID
	f.runCodeEngine = useCodeEngine
	if f.runErr != nil {
		return workflow.RunResult{}, f.runErr
	}
	return workflow.RunResult{SessionID: "session-1", Summary: "done"}, nil
}

func (f *fakeEngine) ApproveAgentTask(context.Context, string, string, string, string) (codeengine.Result, error) {
	return codeengine.Result{Status: "ok"}, nil
}

func TestHealthLiveIsPublic(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedMemoryRequiresBearerToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/memory/session-1", nil)
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWorkflowRunValidatesTask(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/workflow/run", strings.NewReader(`{"task":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWorkflowRunPassesAuthenticatedRequest(t *testing.T) {
	engine := &fakeEngine{}
	s := NewServer(testConfig(), fakeStore{}, engine)
	req := httptest.NewRequest(http.MethodPost, "/workflow/run", strings.NewReader(`{
		"task": "ship auth",
		"session_id": "session-1",
		"use_code_engine": false
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if engine.runTask != "ship auth" {
		t.Fatalf("expected trimmed task to reach engine, got %q", engine.runTask)
	}
	if engine.runSessionID != "session-1" {
		t.Fatalf("expected session id to reach engine, got %q", engine.runSessionID)
	}
	if engine.runCodeEngine {
		t.Fatal("expected use_code_engine=false to reach engine")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["summary"] != "done" {
		t.Fatalf("expected summary response, got %#v", body)
	}
}

func TestReadyReportsStoreFailure(t *testing.T) {
	s := NewServer(testConfig(), fakeStore{err: errors.New("db down")}, &fakeEngine{})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	s.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func newTestServer() *Server {
	return NewServer(testConfig(), fakeStore{}, &fakeEngine{})
}

func testConfig() config.Config {
	return config.Config{WebAuthToken: "test-token"}
}
