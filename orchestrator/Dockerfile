FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates ripgrep nodejs npm \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 agent

# OpenCode/Codex are intentionally best-effort because package names change.
# You can override OPENCODE_BIN/CODEX_BIN and mount your own binaries if needed.
RUN npm install -g opencode-ai @openai/codex || true

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app ./app
USER agent
EXPOSE 8088
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8088"]
