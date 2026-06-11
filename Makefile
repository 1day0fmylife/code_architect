GOCACHE ?= /tmp/go-cache

up:
	docker compose up -d --build

logs:
	docker compose logs -f hermes-brain

pull-model:
	docker compose exec ollama ollama pull qwen2.5-coder:7b

down:
	docker compose down

go-test:
	cd orchestrator && GOCACHE=$(GOCACHE) go test ./...

go-build:
	cd orchestrator && GOCACHE=$(GOCACHE) go build -o /tmp/hermes ./cmd/hermes

check: go-test
