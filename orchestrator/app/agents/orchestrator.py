from uuid import uuid4

from app.adapters.code_engine import run_code_agent
from app.adapters.llm import chat
from app.core.settings import settings
from app.services.config_loader import load_yaml
from app.services.memory import recall, remember


def _format_memory(events) -> str:
    if not events:
        return "No previous memory."
    return "\n".join(f"[{e.role}] {e.content}" for e in events[-12:])


def _agent_prompt(agent: dict, user_task: str, memory_text: str, previous: str) -> str:
    return f"""
You are {agent['title']}.
Mission: {agent['mission']}

Persistent memory:
{memory_text}

User task:
{user_task}

Previous agent outputs:
{previous or 'None'}

Return:
1. Key decisions
2. Concrete actions for repository
3. Risks/blockers
4. Acceptance checks
""".strip()


async def run_workflow(task: str, session_id: str | None = None, use_code_engine: bool = True) -> dict:
    cfg = load_yaml(settings.agents_config)
    session_id = session_id or str(uuid4())
    await remember(session_id, "user", task)
    memory_text = _format_memory(await recall(session_id))

    results = []
    previous = ""
    for name in cfg["workflow"]["default_sequence"]:
        agent = cfg["agents"][name]
        prompt = _agent_prompt(agent, task, memory_text, previous)
        text = await chat(prompt, model=agent.get("model"))
        await remember(session_id, name, text)
        result = {"agent": name, "analysis": text}

        if use_code_engine and name in {"backend", "frontend", "security", "qa"}:
            if settings.require_approval_for_code:
                result["code_engine"] = {
                    "status": "approval_required",
                    "instruction": "Call /approve with the same session_id and concrete agent task to execute OpenCode/Codex."
                }
            else:
                code_prompt = f"Agent: {name}\nTask: {task}\nPlan:\n{text}\nApply safe repository changes and run relevant checks."
                result["code_engine"] = await run_code_agent(code_prompt)
        results.append(result)
        previous += f"\n\n## {name}\n{text}"

    summary = await chat(f"Summarize this multi-agent workflow for operator:\n{previous}")
    await remember(session_id, "summary", summary)
    return {"session_id": session_id, "summary": summary, "results": results}


async def approve_agent_task(session_id: str, agent: str, task: str, engine: str | None = None) -> dict:
    events = await recall(session_id)
    memory_text = _format_memory(events)
    prompt = f"""
You are executing an approved code task.
Agent: {agent}
Task: {task}
Context/memory:
{memory_text}

Work inside repository. Prefer minimal diffs. Run tests/checks when available.
Return changed files, commands run, and risks.
""".strip()
    result = await run_code_agent(prompt, engine=engine)
    await remember(session_id, f"{agent}:code_engine", str(result))
    return result
