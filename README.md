# Hermes Brain + OpenCode Hands

Self-hosted каркас для мультиагентной разработки:

- **Hermes Brain** — Go/Echo v5 оркестратор, память, workflow, Telegram/API управление.
- **OpenCode Hands** — кодовый исполнитель для изменения репозитория.
- **Codex adapter** — опциональный исполнитель вместо OpenCode.
- **Agents** — architect, backend, frontend, security, QA.
- **Agent Memory** — Postgres + pgvector-ready схема, сейчас используется event memory.
- **Models** — Ollama по умолчанию, llama.cpp через compose profile.

## Архитектура

```text
Telegram/API
    ↓
hermes-brain Go + Echo v5
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

Telegram polling теперь встроен в тот же бинарь, что и HTTP API. Отдельный `telegram-bot` контейнер больше не нужен.

## Быстрый старт

```bash
cp .env.example .env
nano .env

docker compose up -d --build
docker compose exec ollama ollama pull qwen2.5-coder:7b
```

По умолчанию наружу публикуются `hermes-brain` на `8088` и web-консоль на `5173`. Postgres, Redis, Ollama и llama.cpp доступны сервисам внутри Docker network; если нужен доступ с хоста, добавь нужные `ports` в `docker-compose.yml`.

Проверка live/readiness:

```bash
curl http://localhost:8088/health/live
curl http://localhost:8088/health/ready
```

`/health/ready` проверяет Postgres, доступность workspace и файл конфигурации агентов.

API workflow защищен bearer token из `WEB_AUTH_TOKEN`:

```bash
export HERMES_TOKEN=change-me-web-token

curl -X POST http://localhost:8088/workflow/run \
  -H "Authorization: Bearer $HERMES_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"task":"Добавь Go/Echo endpoint /healthz и тесты", "use_code_engine": true}'
```

Ответ `/workflow/run` содержит `run_id` и `session_id`. `run_id` сохраняется в Postgres в `workflow_runs` и связывает последующие approval/code-engine records.

По умолчанию `REQUIRE_APPROVAL_FOR_CODE=true`, поэтому backend/frontend/security/QA только подготовят план и вернут `approval_id`. Для запуска кодового агента используй этот `approval_id`:

```bash
curl -X POST http://localhost:8088/workflow/approve \
  -H "Authorization: Bearer $HERMES_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"approval_id":"<approval_id>","engine":"opencode"}'
```

Approval requests сохраняются в Postgres и не теряются при рестарте `hermes-brain`. Каждый `approval_id` можно использовать только один раз.

Просмотр памяти:

```bash
curl http://localhost:8088/memory/<session_id> \
  -H "Authorization: Bearer $HERMES_TOKEN"
```

Ответ code engine включает `changed_files` и `diff_stat`, если запуск выполнялся внутри git-репозитория. Эти поля также сохраняются в Postgres в `code_engine_runs`, чтобы оператор мог восстановить историю approved-запусков.

Для локальной разработки можно временно отключить API auth:

```env
WEB_AUTH_DISABLED=true
```

## Telegram

Telegram polling встроен в `hermes-brain`: отдельный bot service запускать не нужно.

1. Создай бота в Telegram:
   - открой `@BotFather`;
   - выполни `/newbot`;
   - задай имя и username;
   - скопируй bot token в `TELEGRAM_BOT_TOKEN`.
2. Узнай свой Telegram user id:
   - напиши любому id-боту, например `@userinfobot`;
   - скопируй числовой id в `TELEGRAM_ALLOWED_USER_IDS`.
3. Заполни `.env`:

```env
TELEGRAM_BOT_TOKEN=123456:replace-me
TELEGRAM_ALLOWED_USER_IDS=123456789
```

Несколько операторов можно указать через запятую:

```env
TELEGRAM_ALLOWED_USER_IDS=123456789,987654321
```

Если `TELEGRAM_BOT_TOKEN` пустой, Telegram-интеграция не запускается. Если `TELEGRAM_ALLOWED_USER_IDS` пустой, бот принимает команды от любого пользователя, что подходит только для локального стенда.

После изменения `.env` пересоздай контейнер:

```bash
docker compose up -d --build hermes-brain
docker compose logs -f hermes-brain
```

Команды:

```text
/start - открыть главное меню
/menu - открыть главное меню
/help - показать команды
/status - показать состояние бота и БД
/task <описание задачи> - запустить workflow агентов
/approve <approval_id> [opencode|codex] - выполнить approved code-engine задачу
/memory <session_id> - показать последние события памяти
```

Бот также показывает inline-кнопки:

```text
New task - подсказка для /task
Approve - подсказка для /approve
Memory - подсказка для /memory
Status - состояние бота и БД
Help - список команд
Menu - возврат в главное меню
```

Telegram-команды вызывают workflow напрямую внутри `hermes-brain`, поэтому `WEB_AUTH_TOKEN` нужен только для HTTP API.

Быстрая проверка:

1. Напиши боту `/start`.
2. Отправь `/task Проверь структуру проекта и предложи следующий шаг`.
3. Если задача требует кода, возьми `approval_id` из ответа и выполни `/approve <approval_id>`.

Не добавляй bot token в задачи, prompt, issue или сообщения агентам: code engine stdout/stderr маскируется частично, но секреты лучше не передавать в workflow вообще.

Если бот не отвечает:

```bash
docker compose ps
docker compose logs -f hermes-brain
```

Проверь, что `hermes-brain` запущен, `TELEGRAM_BOT_TOKEN` не пустой, твой user id входит в `TELEGRAM_ALLOWED_USER_IDS`, а в логах нет ошибок Telegram API. При старте polling бот вызывает `deleteWebhook`, поэтому старый webhook не должен блокировать `getUpdates`.

## Web-консоль

Frontend находится в `frontend/` и использует тот же `WEB_AUTH_TOKEN`, что HTTP API.

Локальный запуск:

```bash
cd frontend
bun install --frozen-lockfile
bun run dev
```

Docker-запуск:

```bash
docker compose up -d --build frontend
```

Открой `http://localhost:5173`, укажи API URL `http://localhost:8088` и значение `WEB_AUTH_TOKEN`. Консоль умеет проверять health endpoints, запускать `/workflow/run`, выполнять `/workflow/approve` и читать `/memory/<session_id>`.

## Go-разработка

Orchestrator теперь находится в `orchestrator/` как Go module:

```text
orchestrator/
  cmd/hermes/main.go
  internal/auth
  internal/codeengine
  internal/config
  internal/httpapi
  internal/llm
  internal/memory
  internal/telegram
  internal/workflow
```

Локальная сборка:

```bash
cd orchestrator
go mod download
go build ./cmd/hermes
go test ./...
```

Через Makefile из корня:

```bash
make go-build
make go-test
make check
```

Основные переменные окружения:

```env
DATABASE_URL=postgres://agent:change-me@localhost:5432/agent_memory
WEB_AUTH_TOKEN=change-me-web-token
WEB_AUTH_DISABLED=false
DEFAULT_LLM_BACKEND=ollama
DEFAULT_MODEL=qwen2.5-coder:7b
OLLAMA_BASE_URL=http://localhost:11434
LLAMACPP_BASE_URL=http://localhost:8080/v1
WORKSPACE_DIR=/workspace
AGENTS_CONFIG=/app/config/agents.yaml
CODE_ENGINE=opencode
OPENCODE_BIN=opencode
CODEX_BIN=codex
CODE_ENGINE_TIMEOUT_SECONDS=1800
```

## Подключение проекта

Положи репозиторий в `./workspace`:

```bash
git clone https://github.com/1day0fmylife/code_architect.git workspace
```

Или замени volume в `docker-compose.yml` на путь к существующему проекту:

```yaml
volumes:
  - /path/to/project:/workspace
```

## Ollama

Ollama поддерживает локальный API и OpenAI-compatible endpoint. В этом проекте основной вызов идет через `/api/chat`.

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

В Docker image CLI ставятся через npm в отдельном `code-engine-cli` stage, поэтому слой с `opencode-ai` и `@openai/codex` кэшируется отдельно от сборки Go-бинаря:

```text
npm install -g opencode-ai @openai/codex
```

Проверить наличие бинарей можно так:

```bash
docker run --rm hermes-opencode-team-hermes-brain:latest \
  sh -lc 'command -v opencode && command -v codex'
```

Если нужно закрепить свою версию CLI или конкретный путь, задай бинарь явно:

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
orchestrator/internal/codeengine/runner.go
```

Команды по умолчанию:

```text
opencode run <prompt>
codex exec <prompt>
```

Если у твоей версии CLI другой non-interactive синтаксис, поменяй только этот файл.

## Security defaults

- HTTP workflow API защищен bearer token из `WEB_AUTH_TOKEN`.
- Workflow runs, approval requests и code engine runs хранятся в Postgres.
- Кодовые изменения требуют явного `/approve`.
- `approval_id` хранится в Postgres и одноразово переводится из `pending` в `used`.
- Telegram ограничивается `TELEGRAM_ALLOWED_USER_IDS`.
- Репозиторий монтируется только в `/workspace`.
- OpenCode/Codex запускаются внутри контейнера.
- stdout/stderr code engine проходят базовую redaction-маскировку токенов и паролей.
- Ответ code engine включает список измененных файлов и `git diff --stat`, когда workspace является git-репозиторием.
- Базовая threat model описана в `docs/threat-model.md`.
- Секреты не хранятся в памяти явно; не передавай `.env` и токены в задачи.

## Что доработать для production

1. Добавить полноценные embeddings в memory через pgvector.
2. Добавить persisted workflow/approval/code-run tables вместо только event memory.
3. Добавить git branch-per-task и автоматический PR.
4. Добавить approval UI вместо Telegram-only approval.
5. Запретить shell/network для code engine через sandboxing/firejail/gVisor.
6. Добавить audit log и RBAC операторов.
