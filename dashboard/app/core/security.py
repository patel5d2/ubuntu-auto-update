from fastapi import Security, HTTPException
from fastapi.security.api_key import APIKeyHeader
from starlette.status import HTTP_403_FORBIDDEN

from .settings import settings

api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)

def get_api_key(api_key_header: str = Security(api_key_header)):
    if not settings.api_key:
        raise HTTPException(
            status_code=HTTP_403_FORBIDDEN, detail="API key not configured on server"
        )
    if api_key_header != settings.api_key:
        raise HTTPException(
            status_code=HTTP_403_FORBIDDEN, detail="Invalid or missing API key"
        )
    return api_key_header
