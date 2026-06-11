import asyncio
import os
from pathlib import Path

from app.core.settings import settings


class CodeEngineError(RuntimeError):
    pass


async def run_code_agent(prompt: str, engine: str | None = None, timeout_sec: int = 1800) -> dict:
    engine = engine or settings.code_engine
    cwd = Path(settings.workspace_dir)
    cwd.mkdir(parents=True, exist_ok=True)

    if engine == "opencode":
        bin_name = os.getenv("OPENCODE_BIN", settings.opencode_bin)
        # Common non-interactive wrappers differ between releases; override OPENCODE_BIN if needed.
        cmd = [bin_name, "run", prompt]
    elif engine == "codex":
        bin_name = os.getenv("CODEX_BIN", settings.codex_bin)
        cmd = [bin_name, "exec", prompt]
    else:
        raise CodeEngineError(f"Unsupported code engine: {engine}")

    proc = await asyncio.create_subprocess_exec(
        *cmd,
        cwd=str(cwd),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=timeout_sec)
    except asyncio.TimeoutError:
        proc.kill()
        raise CodeEngineError(f"{engine} timed out after {timeout_sec}s")

    return {
        "engine": engine,
        "cmd": " ".join(cmd[:2]) + " <prompt>",
        "returncode": proc.returncode,
        "stdout": stdout.decode(errors="replace")[-20000:],
        "stderr": stderr.decode(errors="replace")[-12000:],
    }
