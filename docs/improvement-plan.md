# Hermes OpenCode Team: improvement plan

Дата анализа: 2026-06-11
Последнее обновление: 2026-06-12

## Краткое состояние

Проект представляет собой ранний self-hosted каркас мультиагентной разработки:

- Go/Echo v5-сервис `hermes-brain` принимает задачи, запускает цепочку агентов и хранит события в Postgres.
- Telegram-бот дает операторский интерфейс для `/task`, `/approve`, `/memory`, `/status`, меню и inline-кнопок.
- Минимальная web-консоль запускает workflow, approve и memory-запросы через bearer token.
- Адаптеры LLM поддерживают Ollama и llama.cpp.
- Кодовый исполнитель вызывает OpenCode или Codex CLI внутри контейнера и работает в смонтированном `/workspace`.
- Конфигурация агентов вынесена в `config/agents.yaml`.
- Docker Compose поднимает orchestrator, frontend, Postgres с pgvector, Redis, Ollama и опциональный llama.cpp.
- Web/API endpoints защищены bearer token, кроме health endpoints; dev-mode без auth управляется явной настройкой.

Главный вывод: архитектура понятная и расширяемая, но проект пока ближе к прототипу. Самые важные зоны улучшения: безопасность исполнения кода, наблюдаемость, тесты, устойчивость workflow, управление схемой БД и полноценная память.

## Уже выполнено

- Orchestrator перенесен на Go/Echo v5.
- Добавлен bearer auth middleware для web/API endpoints.
- Telegram polling и команды `/task`, `/approve`, `/memory` перенесены в Go-service.
- OpenCode/Codex CLI устанавливаются в Docker image через `npm install -g opencode-ai @openai/codex`.
- Dockerfile больше не скрывает ошибку установки code-engine CLI.
- Добавлен минимальный тестовый контур Go:
  - auth middleware;
  - config/env loading;
  - HTTP API auth/validation/health contract.
  - Telegram command handling and allowlist checks.
- Approval flow теперь выдает `approval_id`, а HTTP/Telegram approve используют его вместо произвольного повторного task.
- Approval requests сохраняются в Postgres и одноразово потребляются из `pending` в `used`.
- Code engine result включает `changed_files` и `diff_stat` для git workspace.
- Code engine runs сохраняются в Postgres вместе с approval/session/agent linkage.
- Workflow runs получают `run_id` и сохраняются в Postgres как `running`/`completed`.
- Frontend-зависимости закреплены точными версиями в `frontend/package.json`.
- Добавлена минимальная web-консоль с auth-настройкой, health check, workflow run, approval и memory view.
- Telegram получил `/menu`, `/help`, inline-кнопки и callback-переходы.
- Telegram получил `/status` для быстрой проверки состояния бота и БД.
- Telegram polling очищает webhook перед `getUpdates` и возвращает ошибку при `ok:false` от Telegram API.
- Dockerfile orchestrator разделен на `runtime-base`, `code-engine-cli` и `runtime` stages для кэширования установки OpenCode/Codex CLI.
- Frontend получил отдельный multi-stage Dockerfile.
- Threat model зафиксирован в `docs/threat-model.md`.
- `/health/ready` проверяет Postgres, workspace и agents config.
- `/health/ready` проверяет Postgres, Redis, текущий LLM backend, workspace и agents config.
- `.env.example` и `.dockerignore` усилены для безопасного локального запуска и Docker build context.
- `make check` запускает `gofmt`-проверку, `go vet` и Go-тесты.
- В compose наружу публикуется только `hermes-brain:8088`; инфраструктурные сервисы остаются внутри Docker network.

## Оценка плана

План в текущем виде пригоден как рабочий roadmap: он правильно ставит первыми проверяемость, approval/sandbox и перевод workflow в управляемое состояние. После переноса на Go ближайший акцент смещается с миграции runtime на формализацию контрактов: API schema, approval model, async workflow state и тестируемая Telegram-интеграция.

Что стоило усилить:

- API security выделить отдельно от sandbox. Bearer auth уже есть, но нужны роли, audit log, token rotation и отдельная модель прав для web/Telegram.
- Prompt-injection threat model. Проект запускает code agents по текстовым заданиям и памяти, поэтому нужно явно защищаться от инструкций из README, issue, logs и пользовательского workspace.
- Disaster recovery. Для Postgres, workflow state, memory и audit log нужны backup/restore и retention policy.
- Model operations. Нужны правила выбора моделей, fallback, лимиты стоимости/ресурсов, quality regression tests для агентских ответов.
- Product boundary. Нужно заранее решить, что является MVP: Telegram-first operator loop или web UI-first workflow console.

Обновленная рекомендация: ближайший фокус оставить на Foundation PR, но сразу включить туда API auth skeleton и threat-model документ. Это дешево сейчас и дорого переделывать позже.

## Найденные риски и пробелы

### 1. Безопасность исполнения

- `/workflow/approve` принимает `agent`, `task` и `engine`, но не проверяет, что агент разрешен, задача связана с ранее созданным workflow, а оператор имеет право на запуск.
- API имеет bearer auth, но пока нет ролевой модели, scoped tokens, expiration и audit trail.
- `REQUIRE_APPROVAL_FOR_SHELL` объявлен, но фактически не применяется.
- OpenCode/Codex запускаются в контейнере, однако нет отдельной sandbox-политики для сети, shell-команд, путей записи, лимитов CPU/RAM и allowlist команд.
- Ответ code engine возвращает stdout/stderr почти напрямую; возможна утечка секретов из логов.
- Нет threat model для prompt injection, malicious repository content и tool-output injection.

### 2. Надежность workflow

- Workflow выполняется синхронно внутри HTTP-запроса. Длинные задачи могут упираться в таймауты клиента, Telegram или reverse proxy.
- Нет статусов задач, очереди, отмены, retry и восстановления после падения сервиса.
- Сводка зависит от финального LLM-вызова; если он падает, весь workflow может завершиться ошибкой после уже выполненных шагов.
- `previous` и memory могут быстро разрастаться без token budgeting и summarization.

### 3. Память и база данных

- `scripts/init.sql` создает только расширение `vector`; таблицы создаются через `Base.metadata.create_all`.
- Нет Alembic-миграций, версионирования схемы и индексов под реальные запросы.
- pgvector заявлен как будущая возможность, но embeddings, retrieval и summary memory пока не реализованы.
- Redis подключен в compose и settings, но не используется.

### 4. Качество кода и тесты

- Базовые Go-тесты есть, но покрытие пока минимальное.
- Нет контрактных тестов для memory, LLM adapters, code engine и persisted approval flow.
- Нет линтера, форматтера и CI для Go-модуля.
- Нет централизованной обработки ошибок и структурированных ответов для LLM/code-engine failures.

### 5. Конфигурация и эксплуатация

- `.env.example` есть, но пример пароля слабый и выглядит как реальное значение.
- Нет health/readiness разделения: `/health` не проверяет Postgres, Redis, Ollama и доступность workspace.
- Dockerfile ставит OpenCode/Codex строго, но версии npm-пакетов пока не закреплены.
- Нет pinned npm package versions, SBOM или dependency scanning.

### 6. UX оператора

- Telegram-команды уже имеют меню и inline-кнопки, но пока не показывают прогресс по агентам.
- Approval требует вручную повторять задачу, что повышает риск выполнить не тот prompt.
- Нет веб-интерфейса, истории workflow, diff preview, approve/reject по конкретным proposed changes.

### 7. Восстановление и lifecycle данных

- Нет backup/restore процедуры для Postgres volume.
- Нет retention policy для memory events, code engine logs и audit trails.
- Нет разделения audit log и рабочей памяти: долгоживущие записи безопасности должны храниться иначе, чем prompt context.
- Нет documented upgrade path для схемы БД и конфигурации агентов.

### 8. Model operations

- Нет оценки качества моделей на типовых задачах проекта.
- Нет fallback-стратегии, если Ollama/llama.cpp недоступны или модель отвечает невалидно.
- Нет лимитов на размер prompt, число шагов, повторные попытки и общий бюджет workflow.
- Нет regression set для проверки, что изменения prompts/agents не ухудшают поведение.

## Приоритетный план улучшений

### Этап 1. Базовая надежность и проверяемость

Цель: сделать прототип безопаснее менять и проще проверять.

1. Добавить тестовый контур:
   - unit-тесты для `config`, `memory`, `llm.Client`, `workflow.Engine`;
   - API smoke-тесты для `/health`, `/workflow/run`, `/workflow/approve`, `/memory/{session_id}`.
2. Добавить качество кода:
   - `gofmt`;
   - `go vet`;
   - `staticcheck` или аналогичный Go-linter;
   - `make test`, `make lint`, `make check`.
3. Ввести CI:
   - install dependencies;
   - lint;
   - tests;
   - Docker build smoke.
4. Улучшить error handling:
   - явные ошибки для недоступного LLM/backend;
   - HTTP 400/422 для неверного engine/agent;
   - HTTP 502/504 для downstream failures/timeouts;
   - безопасное логирование без секретов.
5. Добавить минимальный API auth skeleton:
   - bearer token для `/workflow/*` и `/memory/*` уже добавлен;
   - отдельная настройка для dev-mode без auth уже добавлена;
   - тесты на protected endpoints уже добавлены;
   - следующий шаг: роли/scopes для web operator, Telegram operator и internal service calls.
6. Зафиксировать threat model:
   - trusted/untrusted boundaries;
   - prompt injection в workspace files;
   - secrets exposure через logs;
   - network/file-system escape scenarios.

Критерий готовности: локально и в CI проходит `make check`; основные API покрыты тестами; README описывает Go/Echo runtime и auth.

### Этап 2. Approval и sandbox

Цель: превратить approval из текстовой договоренности в enforceable control.

1. Ввести модель workflow run:
   - `workflow_runs`;
   - `agent_steps`;
   - `approval_requests`;
   - `code_engine_runs`.
   - `workflow_runs`, `approval_requests` и `code_engine_runs` уже созданы в Postgres-backed storage;
   - следующий шаг: persisted `agent_steps`.
2. Approval должен ссылаться на конкретный `approval_request_id`, а не принимать произвольный новый task.
   - `approval_id` уже добавлен;
   - approval state уже хранится в Postgres;
   - approval уже связан с persisted `workflow_runs` через `run_id`;
   - следующий шаг: связать approval с persisted `agent_steps`.
3. Валидировать:
   - agent входит в конфиг;
   - engine входит в allowlist;
   - session/run существует;
   - approval еще не использован или явно разрешен repeat.
   - одноразовое использование и engine allowlist уже проверяются.
4. Добавить sandbox policy:
   - readonly mount для исходного repo до approval;
   - отдельная рабочая ветка или копия workspace на task;
   - allowlist путей записи;
   - лимиты timeout/CPU/RAM;
   - опциональное отключение network для code engine;
   - redaction секретов в stdout/stderr.
5. Сохранять diff и список измененных файлов после code-engine run.
   - `changed_files` и `diff_stat` уже возвращаются в API/Telegram result;
   - эти поля уже сохраняются в `code_engine_runs` в Postgres.
6. Добавить prompt hardening:
   - system/developer instructions для code engine не должны смешиваться с untrusted repository content;
   - явно маркировать memory, user task, repo snippets и tool output;
   - запрещать агенту следовать инструкциям из workspace-файлов, если они противоречат operator policy.
   - базовый threat model уже добавлен в `docs/threat-model.md`.

Критерий готовности: невозможно запустить произвольную approved-задачу вне созданного workflow; оператор видит diff/команды перед финальным принятием.

### Этап 3. Асинхронный workflow engine

Цель: убрать длинные операции из request/response и добавить управляемость.

1. Использовать Redis как очередь или добавить легкий job runner:
   - enqueue workflow;
   - worker processes;
   - status polling;
   - retry с ограничениями;
   - cancellation.
2. API:
   - `POST /workflow/run` возвращает `run_id`;
   - `GET /workflow/{run_id}` возвращает статус, шаги, ошибки и summary;
   - `POST /workflow/{run_id}/cancel`;
   - `POST /approvals/{approval_id}/approve`.
3. Telegram:
   - показывать progress по агентам;
   - approval кнопками или коротким approval id;
   - команды `/status`, `/cancel`.

Критерий готовности: длинный workflow не блокирует HTTP-клиента; состояние восстанавливается после рестарта сервиса.

### Этап 4. База данных и память

Цель: сделать память полезной и управляемой.

1. Перейти с `Base.metadata.create_all` на Alembic.
2. Создать миграции для:
   - event memory;
   - workflow runs;
   - approvals;
   - code engine runs;
   - embeddings.
3. Добавить memory layers:
   - raw events;
   - summarized session memory;
   - vector embeddings по событиям/решениям;
   - retrieval по текущей задаче.
4. Добавить token budgeting:
   - ограничение prompt context;
   - rolling summaries;
   - top-k retrieval через pgvector.
5. Добавить lifecycle данных:
   - retention для raw memory и code logs;
   - отдельный immutable audit log;
   - backup/restore runbook;
   - smoke-test восстановления на пустой БД.

Критерий готовности: агентам передается компактный релевантный контекст, а не последние N событий без учета смысла.

### Этап 5. Observability и эксплуатация

Цель: чтобы проект можно было поддерживать в реальной среде.

1. Структурированные JSON-логи с `run_id`, `session_id`, `agent`, `step_id`.
2. Метрики:
   - latency LLM calls;
   - latency code engine;
   - success/failure by agent;
   - queue depth;
   - approval wait time.
3. Health endpoints:
   - `/health/live`;
   - `/health/ready` с проверками Postgres, Redis, workspace, LLM backend.
   - Postgres, Redis, текущий LLM backend, workspace и agents config уже проверяются.
4. Docker hardening:
   - non-root уже есть, сохранить;
   - убрать `npm install ... || true` или вынести CLI в отдельный проверяемый слой;
   - code-engine CLI уже вынесены в отдельный Docker stage;
   - frontend npm/bun package versions уже закреплены;
   - следующий шаг: закрепить версии `opencode-ai` и `@openai/codex`;
   - добавить image build validation.
5. Secret hygiene:
   - улучшить `.env.example`;
   - добавить documented secret redaction;
   - не логировать env и prompts целиком по умолчанию.
6. Model operations:
   - fallback backend/model policy;
   - per-workflow resource budget;
   - small golden set задач для regression checks;
   - metrics по качеству: accepted changes, failed runs, manual rework rate.

Критерий готовности: по логам и метрикам можно понять, где сломался workflow, без доступа к контейнеру.

### Этап 6. Operator UI и интеграции

Цель: сделать управление удобным, а не только технически возможным.

1. Добавить простой web UI:
   - минимальная web-консоль уже добавлена;
   - следующий шаг: список runs;
   - следующий шаг: детали шагов;
   - memory view уже есть в минимальном виде;
   - следующий шаг: approval queue;
   - diff preview;
   - approve/reject.
2. Git integration:
   - branch-per-task;
   - commit metadata с run_id;
   - automatic PR draft;
   - link PR обратно в workflow.
3. Интеграции:
   - GitHub/GitLab;
   - Jira/Linear;
   - MCP adapters;
   - webhook callbacks.

Критерий готовности: оператор может провести задачу от идеи до draft PR без ручного curl и копирования prompt.

## Быстрые победы

Эти задачи можно сделать первыми за короткое время:

1. Добавить CI для `make check`.
2. Валидировать session/run linkage после введения `workflow_runs`.
3. Добавить Redis/LLM checks в `/health/ready`.
   - уже добавлено.
4. Добавить persisted `workflow_runs` и `agent_steps`.
5. Расширить маскировку секретов в stdout/stderr.
6. Сохранять `returncode != 0` как ошибочный статус в persisted code run metadata.
7. Добавить миграционный слой вместо inline `CREATE TABLE IF NOT EXISTS`.
8. Добавить backup/restore runbook.
9. Добавить роли/scopes поверх bearer-token auth.
10. Добавить path allowlist для code engine.

## Предлагаемый порядок ближайшей реализации

1. Foundation PR:
   - Go tests;
   - `make check`;
   - tests for config, API health, engine validation;
   - minimal API auth skeleton;
   - initial threat model document.
2. API hardening PR:
   - typed validators/enums для `agent` и `engine`;
   - централизованные ошибки;
   - redaction helper.
3. Runtime health PR:
   - readiness checks;
   - Dockerfile install validation.
4. Persistence PR:
   - migrate;
   - workflow/approval/code-run tables.
5. Async workflow PR:
   - Redis-backed jobs;
   - status endpoints;
   - Telegram `/status` уже добавлен;
   - следующий шаг: Telegram `/cancel`.
6. Ops baseline PR:
   - backup/restore runbook;
   - retention settings;
   - model fallback policy;
   - regression prompts.

## Definition of Done для production baseline

- `make check` проходит локально и в CI.
- Все внешние вызовы имеют timeout, typed errors и тесты на failure path.
- Approval запускает только заранее созданные approval requests.
- Code engine runs изолированы, логируются, имеют лимиты и redaction.
- Workflow state хранится в БД и переживает рестарт.
- Есть readiness endpoint и structured logs.
- Есть минимальный операторский UX для статуса, approval и просмотра результатов.
- API endpoints защищены хотя бы минимальной аутентификацией.
- Есть threat model и documented mitigation для prompt injection.
- Есть backup/restore процедура для Postgres-backed state.
- Есть модельный regression set для базовых задач агентов.
