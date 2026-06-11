# Hermes Brain + OpenCode Hands

Self-hosted каркас для мультиагентной разработки:

- **Hermes Brain** — оркестратор, память, workflow, Telegram/API управление.
- **OpenCode Hands** — кодовый исполнитель для изменения репозитория.
- **Codex adapter** — опциональный исполнитель вместо OpenCode.
- **Agents** — architect, backend, frontend, security, QA.
- **Agent Memory** — Postgres + pgvector-ready схема, сейчас используется event memory.
- **Models** — Ollama по умолчанию, llama.cpp через compose profile.

## Архитектура

```text
Telegram/API
    ↓
hermes-brain FastAPI
    ↓
Agent workflow
    ├── Architect Agent
    ├── Backend Agent → OpenCode/Codex
    ├── Frontend Agent → OpenCode/Codex
    ├── Security Agent → OpenCode/Codex
    └── QA Agent → OpenCode/Codex
    ↓
Postgres memory + workspace repo
```

## Быстрый старт

```bash
cp .env.example .env
nano .env

docker compose up -d --build

docker compose exec ollama ollama pull qwen2.5-coder:7b
```

Проверка API:

```bash
curl http://localhost:8088/health
```

Запуск workflow:

```bash
curl -X POST http://localhost:8088/workflow/run \
  -H 'Content-Type: application/json' \
  -d '{"task":"Добавь FastAPI endpoint /healthz и тесты", "use_code_engine": true}'
```

По умолчанию `REQUIRE_APPROVAL_FOR_CODE=true`, поэтому backend/frontend/security/QA только подготовят план. Для запуска кодового агента:

```bash
curl -X POST http://localhost:8088/workflow/approve \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"<session_id>","agent":"backend","task":"Реализуй endpoint /healthz и тесты","engine":"opencode"}'
```

## Telegram

В `.env`:

```env
TELEGRAM_BOT_TOKEN=123456:replace-me
TELEGRAM_ALLOWED_USER_IDS=123456789
```

Команды:

```text
/start
/task <описание задачи>
/approve <session_id> <agent> <задача>
/memory <session_id>
```

## Подключение проекта

Положи репозиторий в `./workspace`:

```bash
git clone git@github.com:org/project.git workspace
```

Или замени volume в `docker-compose.yml` на путь к существующему проекту:

```yaml
volumes:
  - /path/to/project:/workspace
```

## Ollama

Ollama поддерживает локальный API и OpenAI-compatible endpoint. В этом проекте основной вызов идет через `/api/chat`, но совместимость с OpenAI endpoint оставлена для внешних инструментов.

Примеры моделей:

```bash
docker compose exec ollama ollama pull qwen2.5-coder:7b
docker compose exec ollama ollama pull deepseek-coder-v2:16b
docker compose exec ollama ollama pull llama3.1:8b
```

## llama.cpp

Скопируй GGUF модель:

```bash
mkdir -p models
cp model.gguf models/model.gguf
```

Запуск с llama.cpp:

```bash
docker compose --profile llamacpp up -d --build
```

В `.env`:

```env
DEFAULT_LLM_BACKEND=llamacpp
DEFAULT_MODEL=local-model
```

## OpenCode/Codex

В контейнере сделана best-effort установка CLI через npm. Если конкретный CLI изменил имя пакета или режим non-interactive, задай бинарь явно:

```env
CODE_ENGINE=opencode
OPENCODE_BIN=/path/to/opencode
```

или:

```env
CODE_ENGINE=codex
CODEX_BIN=/path/to/codex
```

Адаптер находится в:

```text
orchestrator/app/adapters/code_engine.py
```

Команды по умолчанию:

```text
opencode run <prompt>
codex exec <prompt>
```

Если у твоей версии CLI другой non-interactive синтаксис, поменяй только этот файл.

## Security defaults

- Кодовые изменения требуют явного `/approve`.
- Telegram ограничивается `TELEGRAM_ALLOWED_USER_IDS`.
- Репозиторий монтируется только в `/workspace`.
- OpenCode/Codex запускаются внутри контейнера.
- Секреты не хранятся в памяти явно; не передавай `.env` и токены в задачи.

## Что доработать для production

1. Добавить полноценные embeddings в memory через pgvector.
2. Добавить git branch-per-task и автоматический PR.
3. Добавить Kubernetes/GitHub/Jira MCP adapters.
4. Добавить approval UI вместо Telegram-only approval.
5. Запретить shell/network для code engine через sandboxing/firejail/gVisor.
6. Добавить audit log и RBAC операторов.
