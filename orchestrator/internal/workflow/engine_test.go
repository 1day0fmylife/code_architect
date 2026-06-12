package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestConsumeApprovalCanOnlyBeUsedOnce(t *testing.T) {
	engine := &Engine{approvals: map[string]ApprovalRequest{}}
	approval, err := engine.createApproval(context.Background(), "run-1", "session-1", "backend", "approved task")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	got, err := engine.consumeApproval(context.Background(), approval.ID)
	if err != nil {
		t.Fatalf("consume approval: %v", err)
	}
	if got.RunID != "run-1" || got.SessionID != "session-1" || got.Agent != "backend" || got.Task != "approved task" {
		t.Fatalf("unexpected approval: %#v", got)
	}

	_, err = engine.consumeApproval(context.Background(), approval.ID)
	if err == nil {
		t.Fatal("expected second consume to fail")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected already used error, got %v", err)
	}
}

func TestApproveApprovalRejectsUnsupportedEngineWithoutConsuming(t *testing.T) {
	engine := &Engine{
		allowedCode: map[string]struct{}{"opencode": {}, "codex": {}},
		approvals:   map[string]ApprovalRequest{},
	}
	approval, err := engine.createApproval(context.Background(), "run-1", "session-1", "backend", "approved task")
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	_, err = engine.ApproveApproval(context.Background(), approval.ID, "shell")
	if err == nil {
		t.Fatal("expected unsupported engine error")
	}

	stored, ok := engine.approvals[approval.ID]
	if !ok {
		t.Fatal("expected approval to remain stored")
	}
	if stored.Status != "pending" {
		t.Fatalf("expected approval to remain pending, got %q", stored.Status)
	}
}
