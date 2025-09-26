from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from app.core.security import get_api_key

router = APIRouter()

from app.core.config_parser import parse_config, update_config

@router.get("/config-file", dependencies=[Depends(get_api_key)])
async def get_config_file():
    return parse_config()

from app.core.csrf import validate_csrf_token

@router.put("/config-file", dependencies=[Depends(get_api_key), Depends(validate_csrf_token)])
async def update_config_file(config: dict):
    update_config(config)
@router.post("/config-file/restore", dependencies=[Depends(get_api_key), Depends(validate_csrf_token)])
async def restore_default_config():
    with open("/Users/dharminpatel/ubuntu-auto-update/config.default.conf", "r") as f:
        default_config = f.read()
    with open("/Users/dharminpatel/ubuntu-auto-update/config.conf", "w") as f:
        f.write(default_config)
    return {"ok": True}
