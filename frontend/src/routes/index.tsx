import { useState } from "react";
import {
  CheckCircle2,
  Database,
  KeyRound,
  LogOut,
  Play,
  RefreshCw,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { createFileRoute } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import useStore from "@/store";

type RequestState = "idle" | "loading" | "success" | "error";

type WorkflowResult = {
  run_id?: string;
  session_id?: string;
  summary?: string;
  agents?: Array<{ name?: string; summary?: string; approval_id?: string }>;
};

type CodeEngineResult = {
  status?: string;
  stdout?: string;
  stderr?: string;
  changed_files?: string[];
  diff_stat?: string;
};

type MemoryResult = {
  session_id?: string;
  events?: Array<{ role?: string; content?: string; created_at?: string }>;
};

function HermesDashboard() {
  const apiUrl = useStore((state) => state.apiUrl);
  const token = useStore((state) => state.token);
  const isAuthenticated = useStore((state) => state.isAuthenticated);
  const setApiUrl = useStore((state) => state.setApiUrl);
  const setToken = useStore((state) => state.setToken);
  const logout = useStore((state) => state.logout);

  const [tokenInput, setTokenInput] = useState(token);
  const [apiInput, setApiInput] = useState(apiUrl);
  const [task, setTask] = useState("");
  const [sessionID, setSessionID] = useState("");
  const [useCodeEngine, setUseCodeEngine] = useState(true);
  const [approvalID, setApprovalID] = useState("");
  const [engine, setEngine] = useState("opencode");
  const [memorySessionID, setMemorySessionID] = useState("");
  const [health, setHealth] = useState<string>("not checked");
  const [requestState, setRequestState] = useState<RequestState>("idle");
  const [message, setMessage] = useState("");
  const [workflowResult, setWorkflowResult] = useState<WorkflowResult | null>(null);
  const [approvalResult, setApprovalResult] = useState<CodeEngineResult | null>(null);
  const [memoryResult, setMemoryResult] = useState<MemoryResult | null>(null);

  async function request<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(`${apiUrl}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
        ...init?.headers,
      },
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || response.statusText);
    }
    return response.json() as Promise<T>;
  }

  async function runHealthCheck() {
    setRequestState("loading");
    setMessage("");
    try {
      const live = await fetch(`${apiUrl}/health/live`).then((item) => item.json());
      const ready = await fetch(`${apiUrl}/health/ready`).then((item) => item.json());
      setHealth(`live: ${live.status ?? "unknown"}, ready: ${ready.status ?? "unknown"}`);
      setRequestState("success");
    } catch (error) {
      setHealth("unavailable");
      setMessage(error instanceof Error ? error.message : "Health check failed");
      setRequestState("error");
    }
  }

  async function runWorkflow() {
    setRequestState("loading");
    setMessage("");
    setWorkflowResult(null);
    try {
      const result = await request<WorkflowResult>("/workflow/run", {
        method: "POST",
        body: JSON.stringify({
          task,
          session_id: sessionID,
          use_code_engine: useCodeEngine,
        }),
      });
      setWorkflowResult(result);
      setMessage("Workflow finished");
      setRequestState("success");
      if (result.session_id) {
        setMemorySessionID(result.session_id);
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Workflow failed");
      setRequestState("error");
    }
  }

  async function approveWorkflow() {
    setRequestState("loading");
    setMessage("");
    setApprovalResult(null);
    try {
      const result = await request<CodeEngineResult>("/workflow/approve", {
        method: "POST",
        body: JSON.stringify({ approval_id: approvalID, engine }),
      });
      setApprovalResult(result);
      setMessage("Approval executed");
      setRequestState("success");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Approval failed");
      setRequestState("error");
    }
  }

  async function loadMemory() {
    setRequestState("loading");
    setMessage("");
    setMemoryResult(null);
    try {
      const result = await request<MemoryResult>(
        `/memory/${encodeURIComponent(memorySessionID)}?limit=20`,
      );
      setMemoryResult(result);
      setMessage("Memory loaded");
      setRequestState("success");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Memory request failed");
      setRequestState("error");
    }
  }

  function saveAuth() {
    setApiUrl(apiInput);
    setToken(tokenInput);
    setMessage("Auth settings saved");
    setRequestState("success");
  }

  const canRunWorkflow = isAuthenticated && task.trim() !== "";
  const canApprove = isAuthenticated && approvalID.trim() !== "";
  const canLoadMemory = isAuthenticated && memorySessionID.trim() !== "";

  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 py-5 md:px-6">
        <header className="flex flex-col gap-4 border-b pb-5 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Sparkles className="size-4" />
              Hermes Brain
            </div>
            <h1 className="mt-2 text-2xl font-semibold">Code Archestrator Console</h1>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={runHealthCheck}>
              <RefreshCw />
              Health
            </Button>
            <Button variant="outline" onClick={logout} disabled={!isAuthenticated}>
              <LogOut />
              Logout
            </Button>
          </div>
        </header>

        <section className="grid gap-4 lg:grid-cols-[360px_1fr]">
          <aside className="rounded-lg border bg-card p-4">
            <div className="flex items-center gap-2">
              <ShieldCheck className="size-4" />
              <h2 className="text-sm font-semibold">Authentication</h2>
            </div>
            <label className="mt-4 block text-xs font-medium text-muted-foreground" htmlFor="api-url">
              API URL
            </label>
            <input
              id="api-url"
              className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={apiInput}
              onChange={(event) => setApiInput(event.target.value)}
              placeholder="http://localhost:8088"
            />
            <label className="mt-3 block text-xs font-medium text-muted-foreground" htmlFor="token">
              WEB_AUTH_TOKEN
            </label>
            <input
              id="token"
              className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={tokenInput}
              onChange={(event) => setTokenInput(event.target.value)}
              placeholder="Bearer token"
              type="password"
            />
            <Button className="mt-4 w-full" onClick={saveAuth}>
              <KeyRound />
              Save access
            </Button>
            <div className="mt-4 rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              Status: {isAuthenticated ? "token saved" : "token required"}; health: {health}
            </div>
          </aside>

          <section className="grid gap-4">
            <div className="rounded-lg border bg-card p-4">
              <div className="flex items-center gap-2">
                <Play className="size-4" />
                <h2 className="text-sm font-semibold">Workflow</h2>
              </div>
              <textarea
                className="mt-4 min-h-28 w-full rounded-md border bg-background p-3 text-sm"
                value={task}
                onChange={(event) => setTask(event.target.value)}
                placeholder="Describe the task for the agent team"
              />
              <div className="mt-3 grid gap-3 md:grid-cols-[1fr_auto]">
                <input
                  className="h-9 rounded-md border bg-background px-3 text-sm"
                  value={sessionID}
                  onChange={(event) => setSessionID(event.target.value)}
                  placeholder="Optional session_id"
                />
                <label className="flex h-9 items-center gap-2 rounded-md border px-3 text-sm">
                  <input
                    checked={useCodeEngine}
                    onChange={(event) => setUseCodeEngine(event.target.checked)}
                    type="checkbox"
                  />
                  Code engine
                </label>
              </div>
              <Button className="mt-3" disabled={!canRunWorkflow || requestState === "loading"} onClick={runWorkflow}>
                <Play />
                Run workflow
              </Button>
            </div>

            <div className="grid gap-4 xl:grid-cols-2">
              <div className="rounded-lg border bg-card p-4">
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="size-4" />
                  <h2 className="text-sm font-semibold">Approval</h2>
                </div>
                <input
                  className="mt-4 h-9 w-full rounded-md border bg-background px-3 text-sm"
                  value={approvalID}
                  onChange={(event) => setApprovalID(event.target.value)}
                  placeholder="approval_id"
                />
                <select
                  className="mt-3 h-9 w-full rounded-md border bg-background px-3 text-sm"
                  value={engine}
                  onChange={(event) => setEngine(event.target.value)}
                >
                  <option value="opencode">opencode</option>
                  <option value="codex">codex</option>
                </select>
                <Button className="mt-3" disabled={!canApprove || requestState === "loading"} onClick={approveWorkflow}>
                  <CheckCircle2 />
                  Approve
                </Button>
              </div>

              <div className="rounded-lg border bg-card p-4">
                <div className="flex items-center gap-2">
                  <Database className="size-4" />
                  <h2 className="text-sm font-semibold">Memory</h2>
                </div>
                <input
                  className="mt-4 h-9 w-full rounded-md border bg-background px-3 text-sm"
                  value={memorySessionID}
                  onChange={(event) => setMemorySessionID(event.target.value)}
                  placeholder="session_id"
                />
                <Button className="mt-3" disabled={!canLoadMemory || requestState === "loading"} onClick={loadMemory}>
                  <Database />
                  Load memory
                </Button>
              </div>
            </div>
          </section>
        </section>

        {message ? (
          <div
            className={`rounded-md border px-3 py-2 text-sm ${
              requestState === "error" ? "border-destructive text-destructive" : "bg-muted"
            }`}
          >
            {message}
          </div>
        ) : null}

        <section className="grid gap-4 xl:grid-cols-3">
          <ResultBlock title="Workflow result" value={workflowResult} />
          <ResultBlock title="Approval result" value={approvalResult} />
          <ResultBlock title="Memory result" value={memoryResult} />
        </section>
      </div>
    </main>
  );
}

function ResultBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <div className="min-h-48 rounded-lg border bg-card p-4">
      <h2 className="text-sm font-semibold">{title}</h2>
      <pre className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 text-xs">
        {value ? JSON.stringify(value, null, 2) : "No data"}
      </pre>
    </div>
  );
}

export const Route = createFileRoute("/")({
  component: HermesDashboard,
  notFoundComponent: () => <div>Page Not Found</div>,
  errorComponent: ({ error }) => <div>Error: {error.message}</div>,
});
