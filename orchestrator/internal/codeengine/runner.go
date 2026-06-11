package codeengine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hermes-opencode-team/orchestrator/internal/config"
)

type Result struct {
	Engine     string `json:"engine"`
	Command    string `json:"cmd"`
	ReturnCode int    `json:"returncode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Status     string `json:"status"`
}

type Runner struct {
	cfg config.Config
}

func NewRunner(cfg config.Config) Runner {
	return Runner{cfg: cfg}
}

func (r Runner) Run(ctx context.Context, prompt, engine string) (Result, error) {
	if engine == "" {
		engine = r.cfg.CodeEngine
	}
	workspace := filepath.Clean(r.cfg.WorkspaceDir)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return Result{}, err
	}

	var bin string
	var args []string
	switch engine {
	case "opencode":
		bin = r.cfg.OpenCodeBin
		args = []string{"run", prompt}
	case "codex":
		bin = r.cfg.CodexBin
		args = []string{"exec", prompt}
	default:
		return Result{}, fmt.Errorf("unsupported code engine: %s", engine)
	}

	timeout := r.cfg.CodeEngineTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = workspace
	stdout, stderr := strings.Builder{}, strings.Builder{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	returnCode := 0
	if cmd.ProcessState != nil {
		returnCode = cmd.ProcessState.ExitCode()
	}
	status := "ok"
	if err != nil {
		status = "error"
	}
	if runCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		err = fmt.Errorf("%s timed out after %s", engine, timeout)
	}

	result := Result{
		Engine:     engine,
		Command:    bin + " " + args[0] + " <prompt>",
		ReturnCode: returnCode,
		Stdout:     limit(redact(stdout.String()), 20000),
		Stderr:     limit(redact(stderr.String()), 12000),
		Status:     status,
	}
	return result, err
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key)=([^\s]+)`),
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s]+`),
}

func redact(input string) string {
	out := input
	for _, pattern := range secretPatterns {
		out = pattern.ReplaceAllString(out, `${1}[REDACTED]`)
	}
	return out
}

func limit(input string, max int) string {
	if len(input) <= max {
		return input
	}
	return input[len(input)-max:]
}
