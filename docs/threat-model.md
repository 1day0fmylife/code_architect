# Hermes Threat Model

Last updated: 2026-06-12

## Scope

This document covers the current Hermes Brain operator loop:

- HTTP API and Telegram commands.
- Agent workflow prompts and persistent memory.
- OpenCode/Codex code execution in `/workspace`.
- Postgres-backed memory, approval requests, and code engine run records.

## Trust Boundaries

- Operator input is trusted only after API bearer auth or Telegram allowlist checks.
- Repository files, issue text, logs, model output, and tool output are untrusted.
- LLM responses are advisory until an operator approves a concrete `approval_id`.
- Code engine stdout/stderr may contain sensitive data and must be treated as untrusted.
- Postgres is trusted storage, but records can contain untrusted prompt/output content.

## Threats

### Prompt Injection

Untrusted repository content can instruct agents to ignore operator policy, reveal secrets, or execute unsafe commands.

Current mitigations:

- Code execution requires a pre-created `approval_id`.
- Approvals are one-use and persisted in Postgres.
- Operators approve generated code tasks rather than passing arbitrary new task text.
- Prompts separate user task, memory, and agent plan sections.

Remaining gaps:

- No hard prompt policy yet that explicitly marks repository content as untrusted.
- No prompt-size budgeting or retrieval filtering.
- No regression suite for prompt-injection cases.

### Sandbox Escape

OpenCode/Codex run inside the container with the mounted workspace. A malicious or mistaken command could modify unexpected paths, use network access, or consume excessive resources.

Current mitigations:

- Code engine runs inside the `hermes-brain` container.
- Workspace path is fixed by `WORKSPACE_DIR`.
- Code engine has a timeout.
- Changed files and diff stat are reported after execution.

Remaining gaps:

- No read-only pre-approval workspace.
- No allowlist for writable paths.
- No network disable switch for code engine.
- No CPU/memory limits specific to code engine process.
- No branch-per-task or disposable workspace copy.

### Secret Leakage

Secrets can leak through prompts, environment variables, command output, or logs.

Current mitigations:

- API auth token is not required for Telegram commands.
- Code engine stdout/stderr use basic redaction for token/password/API key patterns.
- README instructs operators not to place tokens in tasks or prompts.

Remaining gaps:

- Redaction is pattern-based and incomplete.
- Prompts and memory are not classified by sensitivity.
- No secret scanner before storing code engine output.
- No retention policy for memory or code engine logs.

## Security Invariants

- `/workflow/approve` must require `approval_id`.
- A pending approval must be consumed at most once.
- Unsupported code engines must be rejected before approval consumption.
- Code engine results must record status, return code, changed files, and diff stat when available.
- Health endpoints may be public; workflow and memory endpoints require auth unless dev auth is explicitly disabled.

## Next Mitigations

- Add persisted `workflow_runs` and `agent_steps`.
- Add branch-per-task or disposable workspace copy.
- Add path allowlist and network policy for code engine.
- Add prompt-injection regression prompts.
- Add backup/restore and retention policy for Postgres-backed state.
