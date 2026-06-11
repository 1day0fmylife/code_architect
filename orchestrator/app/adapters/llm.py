import httpx
from app.core.settings import settings


async def chat(prompt: str, model: str | None = None, backend: str | None = None) -> str:
    backend = backend or settings.default_llm_backend
    model = model or settings.default_model

    if backend == "ollama":
        url = f"{settings.ollama_base_url}/api/chat"
        payload = {"model": model, "stream": False, "messages": [{"role": "user", "content": prompt}]}
        async with httpx.AsyncClient(timeout=300) as client:
            r = await client.post(url, json=payload)
            r.raise_for_status()
            return r.json()["message"]["content"]

    if backend == "llamacpp":
        url = f"{settings.llamacpp_base_url.rstrip('/')}/chat/completions"
        payload = {"model": model, "messages": [{"role": "user", "content": prompt}], "temperature": 0.2}
        async with httpx.AsyncClient(timeout=300) as client:
            r = await client.post(url, json=payload)
            r.raise_for_status()
            return r.json()["choices"][0]["message"]["content"]

    raise ValueError(f"Unsupported backend: {backend}")
