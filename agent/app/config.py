from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    GEMINI_API_KEY: str = ""
    ANTHROPIC_API_KEY: str = ""
    VOYAGE_API_KEY: str = ""
    MONGODB_URI: str = "mongodb://localhost:27017"
    MONGODB_DB: str = "clarity"
    COORDINATOR_URL: str = "http://localhost:8080"

    VISION_MODEL: str = "gemini-3.8-flash"
    EXPLAIN_MODEL: str = "gemini-3.8-flash"
    CODEGEN_MODEL: str = "claude-sonnet-5"
    EMBED_MODEL: str = "voyage-code-3"

    LANGSMITH_TRACING: bool = False
    LANGSMITH_API_KEY: str = ""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


settings = Settings()
