up:
	docker compose up -d --build

logs:
	docker compose logs -f hermes-brain telegram-bot

pull-model:
	docker compose exec ollama ollama pull qwen2.5-coder:7b

down:
	docker compose down
