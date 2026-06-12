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

# OpenCode/Codex CLI are required by the code engine runners.
RUN npm install -g opencode-ai @openai/codex

WORKDIR /app
COPY --from=builder /out/hermes /app/hermes
USER agent
EXPOSE 8088
CMD ["/app/hermes"]
