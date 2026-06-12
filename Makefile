GOCACHE ?= /tmp/go-cache
GOMODCACHE ?= /tmp/go-mod-cache

up:
	docker compose up -d --build

logs:
	docker compose logs -f hermes-brain

pull-model:
	docker compose exec ollama ollama pull qwen2.5-coder:7b

down:
	docker compose down

go-test:
	cd orchestrator && GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test ./...

go-build:
	cd orchestrator && GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -o /tmp/hermes ./cmd/hermes

fmt:
	cd orchestrator && gofmt -w cmd internal

fmt-check:
	cd orchestrator && test -z "$$(gofmt -l cmd internal)"

vet:
	cd orchestrator && GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go vet ./...

check: fmt-check vet go-test
