from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    database_url: str
    redis_url: str = "redis://redis:6379/0"
    ollama_base_url: str = "http://ollama:11434"
    llamacpp_base_url: str = "http://llamacpp:8080/v1"
    workspace_dir: str = "/workspace"
    agents_config: str = "/app/config/agents.yaml"
    default_llm_backend: str = "ollama"
    default_model: str = "qwen2.5-coder:7b"
    code_engine: str = "opencode"
    opencode_bin: str = "opencode"
    codex_bin: str = "codex"
    require_approval_for_code: bool = True
    require_approval_for_shell: bool = True
    telegram_bot_token: str | None = None
    telegram_allowed_user_ids: str | None = None

    class Config:
        env_file = ".env"
        extra = "ignore"


settings = Settings()
