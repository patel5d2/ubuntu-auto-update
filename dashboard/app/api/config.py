from fastapi import APIRouter, Depends
from pydantic import BaseModel

from app.core.security import get_api_key
from app.core.settings import settings, Settings

router = APIRouter()

class Config(BaseModel):
    smtp_host: str
    smtp_port: int
    smtp_user: str
    smtp_password: str
    smtp_from: str
    discord_webhook_url: str

@router.get("/config", response_model=Config, dependencies=[Depends(get_api_key)])
async def get_config():
    return settings

from dotenv import set_key

from app.core.csrf import validate_csrf_token

@router.put("/config", dependencies=[Depends(get_api_key), Depends(validate_csrf_token)])
async def update_config(config: Config):
    # Write the configuration to the .env file
    set_key(".env", "SMTP_HOST", config.smtp_host)
    set_key(".env", "SMTP_PORT", str(config.smtp_port))
    set_key(".env", "SMTP_USER", config.smtp_user)
    set_key(".env", "SMTP_PASSWORD", config.smtp_password)
    set_key(".env", "SMTP_FROM", config.smtp_from)
    set_key(".env", "DISCORD_WEBHOOK_URL", config.discord_webhook_url)

    # Update the settings in the running application
    settings.smtp_host = config.smtp_host
    settings.smtp_port = config.smtp_port
    settings.smtp_user = config.smtp_user
    settings.smtp_password = config.smtp_password
    settings.smtp_from = config.smtp_from
    settings.discord_webhook_url = config.discord_webhook_url

    return {"ok": True}
