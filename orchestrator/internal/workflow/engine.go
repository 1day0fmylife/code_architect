package workflow

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"

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
	approvals   map[string]ApprovalRequest
	mu          sync.Mutex
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

type ApprovalRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	Status    string `json:"status"`
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
		approvals:   map[string]ApprovalRequest{},
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
				approval, err := e.createApproval(ctx, id, name, approvedCodeTask(task, text))
				if err != nil {
					return RunResult{}, err
				}
				result.CodeEngine = map[string]string{
					"status":      "approval_required",
					"approval_id": approval.ID,
					"instruction": "Call /workflow/approve with approval_id to run the approved code task.",
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

func (e *Engine) ApproveApproval(ctx context.Context, approvalID, engine string) (codeengine.Result, error) {
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return codeengine.Result{}, fmt.Errorf("approval_id is required")
	}
	if engine != "" {
		if _, ok := e.allowedCode[engine]; !ok {
			return codeengine.Result{}, fmt.Errorf("unsupported code engine: %s", engine)
		}
	}

	approval, err := e.consumeApproval(ctx, approvalID)
	if err != nil {
		return codeengine.Result{}, err
	}
	return e.runApprovedTask(ctx, approval.ID, approval.SessionID, approval.Agent, approval.Task, engine)
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
	return e.runApprovedTask(ctx, "", sessionID, agentName, task, engine)
}

func (e *Engine) runApprovedTask(ctx context.Context, approvalID, sessionID, agentName, task, engine string) (codeengine.Result, error) {
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
	if saveErr := e.saveCodeEngineRun(ctx, approvalID, sessionID, agentName, result); saveErr != nil && err == nil {
		err = saveErr
	}
	if rememberErr := e.memory.Remember(ctx, sessionID, agentName+":code_engine", fmt.Sprintf("%+v", result)); rememberErr != nil && err == nil {
		err = rememberErr
	}
	return result, err
}

func (e *Engine) saveCodeEngineRun(ctx context.Context, approvalID, sessionID, agentName string, result codeengine.Result) error {
	if e.memory == nil {
		return nil
	}
	return e.memory.SaveCodeEngineRun(ctx, memory.CodeEngineRun{
		SessionID:    sessionID,
		ApprovalID:   approvalID,
		Agent:        agentName,
		Engine:       result.Engine,
		Command:      result.Command,
		ReturnCode:   result.ReturnCode,
		Status:       result.Status,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		ChangedFiles: result.ChangedFiles,
		DiffStat:     result.DiffStat,
	})
}

func (e *Engine) createApproval(ctx context.Context, sessionID, agentName, task string) (ApprovalRequest, error) {
	approval := ApprovalRequest{
		ID:        "appr_" + newSessionID(),
		SessionID: sessionID,
		Agent:     agentName,
		Task:      task,
		Status:    "pending",
	}
	if e.memory != nil {
		err := e.memory.SaveApproval(ctx, memory.ApprovalRequest{
			ID:        approval.ID,
			SessionID: approval.SessionID,
			Agent:     approval.Agent,
			Task:      approval.Task,
			Status:    approval.Status,
		})
		return approval, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.approvals[approval.ID] = approval
	return approval, nil
}

func (e *Engine) consumeApproval(ctx context.Context, id string) (ApprovalRequest, error) {
	if e.memory != nil {
		approval, err := e.memory.ConsumeApproval(ctx, id)
		if err != nil {
			return ApprovalRequest{}, fmt.Errorf("unknown or already used approval_id: %s", id)
		}
		return ApprovalRequest{
			ID:        approval.ID,
			SessionID: approval.SessionID,
			Agent:     approval.Agent,
			Task:      approval.Task,
			Status:    approval.Status,
		}, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	approval, ok := e.approvals[id]
	if !ok {
		return ApprovalRequest{}, fmt.Errorf("unknown approval_id: %s", id)
	}
	if approval.Status != "pending" {
		return ApprovalRequest{}, fmt.Errorf("approval_id is already used: %s", id)
	}
	approval.Status = "used"
	e.approvals[id] = approval
	return approval, nil
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

func approvedCodeTask(userTask, agentPlan string) string {
	return fmt.Sprintf("Apply the approved plan for user task:\n%s\n\nApproved agent plan:\n%s", userTask, agentPlan)
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
