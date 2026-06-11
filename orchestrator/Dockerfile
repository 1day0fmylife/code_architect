FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/hermes ./cmd/hermes

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git curl ripgrep nodejs npm \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 agent

# OpenCode/Codex are intentionally best-effort because package names and
# non-interactive modes may differ between releases. Override OPENCODE_BIN or
# CODEX_BIN when mounting a known-good binary.
RUN npm install -g opencode-ai @openai/codex || true

WORKDIR /app
COPY --from=builder /out/hermes /app/hermes
USER agent
EXPOSE 8088
CMD ["/app/hermes"]
