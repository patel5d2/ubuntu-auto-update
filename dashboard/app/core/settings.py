from pydantic import BaseSettings

import secrets

class Settings(BaseSettings):
    app_name: str = "Ubuntu Auto-Update Dashboard"
    app_version: str = "1.0.0"
    database_url: str = "sqlite:///./dashboard.db"
    api_key: str = secrets.token_urlsafe(32)
    smtp_host: str = ""
    smtp_port: int = 587
    smtp_user: str = ""
    smtp_password: str = ""
    smtp_from: str = ""
    discord_webhook_url: str = ""

    class Config:
        env_file = ".env"

settings = Settings()
