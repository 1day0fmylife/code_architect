package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRequiresAuthTokenUnlessDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:pass@localhost:5432/agent_memory")
	t.Setenv("WEB_AUTH_TOKEN", "")
	t.Setenv("WEB_AUTH_DISABLED", "false")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing WEB_AUTH_TOKEN error")
	}
}

func TestLoadAllowsDisabledAuth(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:pass@localhost:5432/agent_memory")
	t.Setenv("WEB_AUTH_TOKEN", "")
	t.Setenv("WEB_AUTH_DISABLED", "true")
	t.Setenv("CODE_ENGINE_TIMEOUT_SECONDS", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if !cfg.WebAuthDisabled {
		t.Fatal("expected auth to be disabled")
	}
	if cfg.CodeEngineTimeout != 42*time.Second {
		t.Fatalf("expected 42s timeout, got %s", cfg.CodeEngineTimeout)
	}
}

func TestLoadAgentsExpandsEnvironment(t *testing.T) {
	t.Setenv("DEFAULT_MODEL", "test-model")
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.yaml")
	err := os.WriteFile(path, []byte(`
project:
  name: test
workflow:
  default_sequence:
    - architect
agents:
  architect:
    title: Architect
    model: ${DEFAULT_MODEL:-fallback-model}
    mission: Plan work.
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadAgents(path)
	if err != nil {
		t.Fatalf("load agents: %v", err)
	}
	if cfg.Agents["architect"].Model != "test-model" {
		t.Fatalf("expected expanded model, got %q", cfg.Agents["architect"].Model)
	}
}
