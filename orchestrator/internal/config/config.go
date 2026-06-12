package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Addr                    string
	DatabaseURL             string
	RedisURL                string
	OllamaBaseURL           string
	LlamaCPPBaseURL         string
	WorkspaceDir            string
	AgentsConfigPath        string
	DefaultLLMBackend       string
	DefaultModel            string
	CodeEngine              string
	OpenCodeBin             string
	CodexBin                string
	RequireApprovalForCode  bool
	RequireApprovalForShell bool
	CodeEngineTimeout       time.Duration
	WebAuthToken            string
	WebAuthDisabled         bool
	TelegramBotToken        string
	TelegramAllowedUserIDs  map[int64]struct{}
	TelegramDropPending     bool
}

type AgentsConfig struct {
	Project struct {
		Name          string `yaml:"name"`
		DefaultBranch string `yaml:"default_branch"`
		Workspace     string `yaml:"workspace"`
	} `yaml:"project"`
	LLM struct {
		DefaultBackend string `yaml:"default_backend"`
		DefaultModel   string `yaml:"default_model"`
	} `yaml:"llm"`
	Agents   map[string]Agent `yaml:"agents"`
	Workflow struct {
		DefaultSequence []string `yaml:"default_sequence"`
	} `yaml:"workflow"`
}

type Agent struct {
	Title   string `yaml:"title"`
	Model   string `yaml:"model"`
	Mission string `yaml:"mission"`
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                    env("HTTP_ADDR", ":8088"),
		DatabaseURL:             env("DATABASE_URL", ""),
		RedisURL:                env("REDIS_URL", "redis://redis:6379/0"),
		OllamaBaseURL:           env("OLLAMA_BASE_URL", "http://ollama:11434"),
		LlamaCPPBaseURL:         env("LLAMACPP_BASE_URL", "http://llamacpp:8080/v1"),
		WorkspaceDir:            env("WORKSPACE_DIR", "/workspace"),
		AgentsConfigPath:        env("AGENTS_CONFIG", "/app/config/agents.yaml"),
		DefaultLLMBackend:       env("DEFAULT_LLM_BACKEND", "ollama"),
		DefaultModel:            env("DEFAULT_MODEL", "qwen2.5-coder:7b"),
		CodeEngine:              env("CODE_ENGINE", "opencode"),
		OpenCodeBin:             env("OPENCODE_BIN", "opencode"),
		CodexBin:                env("CODEX_BIN", "codex"),
		RequireApprovalForCode:  envBool("REQUIRE_APPROVAL_FOR_CODE", true),
		RequireApprovalForShell: envBool("REQUIRE_APPROVAL_FOR_SHELL", true),
		CodeEngineTimeout:       time.Duration(envInt("CODE_ENGINE_TIMEOUT_SECONDS", 1800)) * time.Second,
		WebAuthToken:            env("WEB_AUTH_TOKEN", ""),
		WebAuthDisabled:         envBool("WEB_AUTH_DISABLED", false),
		TelegramBotToken:        env("TELEGRAM_BOT_TOKEN", ""),
		TelegramAllowedUserIDs:  parseAllowedUsers(env("TELEGRAM_ALLOWED_USER_IDS", "")),
		TelegramDropPending:     envBool("TELEGRAM_DROP_PENDING_UPDATES_ON_START", false),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if !cfg.WebAuthDisabled && cfg.WebAuthToken == "" {
		return cfg, fmt.Errorf("WEB_AUTH_TOKEN is required unless WEB_AUTH_DISABLED=true")
	}
	return cfg, nil
}

func LoadAgents(path string) (AgentsConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AgentsConfig{}, err
	}
	expanded := expandEnv(string(raw))
	var cfg AgentsConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return AgentsConfig{}, err
	}
	if len(cfg.Workflow.DefaultSequence) == 0 {
		return AgentsConfig{}, fmt.Errorf("workflow.default_sequence is empty")
	}
	return cfg, nil
}

func (c Config) AgentNames() (map[string]struct{}, error) {
	agents, err := LoadAgents(c.AgentsConfigPath)
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(agents.Agents))
	for name := range agents.Agents {
		names[name] = struct{}{}
	}
	return names, nil
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseAllowedUsers(raw string) map[int64]struct{} {
	allowed := map[int64]struct{}{}
	for _, part := range strings.Split(strings.ReplaceAll(raw, " ", ""), ",") {
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err == nil {
			allowed[id] = struct{}{}
		}
	}
	return allowed
}

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(?::-(.*?))?}`)

func expandEnv(input string) string {
	return envPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		if value := os.Getenv(parts[1]); value != "" {
			return value
		}
		return parts[2]
	})
}
