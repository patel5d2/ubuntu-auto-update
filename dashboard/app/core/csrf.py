from fastapi import Request, HTTPException
from itsdangerous import URLSafeTimedSerializer

from app.core.settings import settings

serializer = URLSafeTimedSerializer(settings.api_key)

def get_csrf_token():
    return serializer.dumps("csrf")

def validate_csrf_token(request: Request):
    csrf_token = request.headers.get("X-CSRF-Token")
    if not csrf_token:
        raise HTTPException(status_code=400, detail="CSRF token missing")
    try:
        serializer.loads(csrf_token, max_age=3600)
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid CSRF token")
