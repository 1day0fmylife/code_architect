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
	Engine       string   `json:"engine"`
	Command      string   `json:"cmd"`
	ReturnCode   int      `json:"returncode"`
	Stdout       string   `json:"stdout"`
	Stderr       string   `json:"stderr"`
	Status       string   `json:"status"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	DiffStat     string   `json:"diff_stat,omitempty"`
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
	changedFiles, diffStat := collectGitChanges(ctx, workspace)

	result := Result{
		Engine:       engine,
		Command:      bin + " " + args[0] + " <prompt>",
		ReturnCode:   returnCode,
		Stdout:       limit(redact(stdout.String()), 20000),
		Stderr:       limit(redact(stderr.String()), 12000),
		Status:       status,
		ChangedFiles: changedFiles,
		DiffStat:     limit(redact(diffStat), 12000),
	}
	return result, err
}

func collectGitChanges(ctx context.Context, workspace string) ([]string, string) {
	statusOut, err := gitOutput(ctx, workspace, "status", "--short", "--untracked-files=all")
	if err != nil {
		return nil, ""
	}
	files := parseGitStatusFiles(statusOut)
	diffStat, _ := gitOutput(ctx, workspace, "diff", "--stat")
	return files, strings.TrimSpace(diffStat)
}

func gitOutput(ctx context.Context, workspace string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = workspace
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseGitStatusFiles(status string) []string {
	seen := map[string]struct{}{}
	files := []string{}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == "" {
			continue
		}
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = parts[len(parts)-1]
		}
		path = strings.Trim(path, `"`)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	return files
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
