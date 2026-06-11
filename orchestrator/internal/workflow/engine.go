package workflow

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"hermes-opencode-team/orchestrator/internal/codeengine"
	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/llm"
	"hermes-opencode-team/orchestrator/internal/memory"
)

type Engine struct {
	cfg         config.Config
	agents      config.AgentsConfig
	memory      *memory.Store
	llm         llm.Client
	code        codeengine.Runner
	agentNames  map[string]struct{}
	allowedCode map[string]struct{}
}

type AgentResult struct {
	Agent      string `json:"agent"`
	Analysis   string `json:"analysis"`
	CodeEngine any    `json:"code_engine,omitempty"`
}

type RunResult struct {
	SessionID string        `json:"session_id"`
	Summary   string        `json:"summary"`
	Results   []AgentResult `json:"results"`
}

func NewEngine(cfg config.Config, store *memory.Store) (*Engine, error) {
	agents, err := config.LoadAgents(cfg.AgentsConfigPath)
	if err != nil {
		return nil, err
	}
	names := map[string]struct{}{}
	for name := range agents.Agents {
		names[name] = struct{}{}
	}
	return &Engine{
		cfg:         cfg,
		agents:      agents,
		memory:      store,
		llm:         llm.NewClient(cfg),
		code:        codeengine.NewRunner(cfg),
		agentNames:  names,
		allowedCode: map[string]struct{}{"opencode": {}, "codex": {}},
	}, nil
}

func (e *Engine) RunWorkflow(ctx context.Context, task, sessionID string, useCodeEngine bool) (RunResult, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		id = newSessionID()
	}
	if err := e.memory.Remember(ctx, id, "user", task); err != nil {
		return RunResult{}, err
	}
	events, err := e.memory.Recall(ctx, id, 20)
	if err != nil {
		return RunResult{}, err
	}
	memoryText := formatMemory(events)

	results := []AgentResult{}
	previous := ""
	for _, name := range e.agents.Workflow.DefaultSequence {
		agent, ok := e.agents.Agents[name]
		if !ok {
			return RunResult{}, fmt.Errorf("workflow references unknown agent: %s", name)
		}
		prompt := agentPrompt(agent, task, memoryText, previous)
		text, err := e.llm.Chat(ctx, prompt, agent.Model, "")
		if err != nil {
			return RunResult{}, fmt.Errorf("%s llm call: %w", name, err)
		}
		if err := e.memory.Remember(ctx, id, name, text); err != nil {
			return RunResult{}, err
		}
		result := AgentResult{Agent: name, Analysis: text}
		if useCodeEngine && isCodeAgent(name) {
			if e.cfg.RequireApprovalForCode {
				result.CodeEngine = map[string]string{
					"status":      "approval_required",
					"instruction": "Call /workflow/approve with the same session_id and a concrete approved agent task.",
				}
			} else {
				codePrompt := fmt.Sprintf("Agent: %s\nTask: %s\nPlan:\n%s\nApply safe repository changes and run relevant checks.", name, task, text)
				codeResult, runErr := e.code.Run(ctx, codePrompt, "")
				result.CodeEngine = codeResult
				if runErr != nil {
					result.CodeEngine = codeResult
				}
			}
		}
		results = append(results, result)
		previous += fmt.Sprintf("\n\n## %s\n%s", name, text)
	}

	summary, err := e.llm.Chat(ctx, "Summarize this multi-agent workflow for operator:\n"+previous, "", "")
	if err != nil {
		return RunResult{}, err
	}
	if err := e.memory.Remember(ctx, id, "summary", summary); err != nil {
		return RunResult{}, err
	}
	return RunResult{SessionID: id, Summary: summary, Results: results}, nil
}

func (e *Engine) ApproveAgentTask(ctx context.Context, sessionID, agentName, task, engine string) (codeengine.Result, error) {
	if _, ok := e.agentNames[agentName]; !ok {
		return codeengine.Result{}, fmt.Errorf("unknown agent: %s", agentName)
	}
	if engine != "" {
		if _, ok := e.allowedCode[engine]; !ok {
			return codeengine.Result{}, fmt.Errorf("unsupported code engine: %s", engine)
		}
	}
	events, err := e.memory.Recall(ctx, sessionID, 20)
	if err != nil {
		return codeengine.Result{}, err
	}
	prompt := fmt.Sprintf(`You are executing an approved code task.
Agent: %s
Task: %s
Context/memory:
%s

Work inside repository. Prefer minimal diffs. Run tests/checks when available.
Return changed files, commands run, and risks.`, agentName, task, formatMemory(events))
	result, err := e.code.Run(ctx, prompt, engine)
	if rememberErr := e.memory.Remember(ctx, sessionID, agentName+":code_engine", fmt.Sprintf("%+v", result)); rememberErr != nil && err == nil {
		err = rememberErr
	}
	return result, err
}

func (e *Engine) IsKnownAgent(name string) bool {
	_, ok := e.agentNames[name]
	return ok
}

func formatMemory(events []memory.Event) string {
	if len(events) == 0 {
		return "No previous memory."
	}
	start := 0
	if len(events) > 12 {
		start = len(events) - 12
	}
	lines := make([]string, 0, len(events)-start)
	for _, event := range events[start:] {
		lines = append(lines, fmt.Sprintf("[%s] %s", event.Role, event.Content))
	}
	return strings.Join(lines, "\n")
}

func agentPrompt(agent config.Agent, userTask, memoryText, previous string) string {
	if previous == "" {
		previous = "None"
	}
	return fmt.Sprintf(`You are %s.
Mission: %s

Persistent memory:
%s

User task:
%s

Previous agent outputs:
%s

Return:
1. Key decisions
2. Concrete actions for repository
3. Risks/blockers
4. Acceptance checks`, agent.Title, agent.Mission, memoryText, userTask, previous)
}

func isCodeAgent(name string) bool {
	switch name {
	case "backend", "frontend", "security", "qa":
		return true
	default:
		return false
	}
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "session"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
