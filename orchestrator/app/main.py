from fastapi import FastAPI
from pydantic import BaseModel

from app.agents.orchestrator import approve_agent_task, run_workflow
from app.services.memory import init_db, recall

app = FastAPI(title="Hermes Brain + OpenCode Hands")


class TaskRequest(BaseModel):
    task: str
    session_id: str | None = None
    use_code_engine: bool = True


class ApproveRequest(BaseModel):
    session_id: str
    agent: str
    task: str
    engine: str | None = None


@app.on_event("startup")
async def startup() -> None:
    await init_db()


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.post("/workflow/run")
async def workflow_run(req: TaskRequest) -> dict:
    return await run_workflow(req.task, req.session_id, req.use_code_engine)


@app.post("/workflow/approve")
async def workflow_approve(req: ApproveRequest) -> dict:
    return await approve_agent_task(req.session_id, req.agent, req.task, req.engine)


@app.get("/memory/{session_id}")
async def memory(session_id: str) -> dict:
    events = await recall(session_id, limit=50)
    return {"session_id": session_id, "events": [{"role": e.role, "content": e.content, "created_at": e.created_at.isoformat()} for e in events]}
