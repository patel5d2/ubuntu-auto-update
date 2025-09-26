from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
import bleach

class XSSMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        if request.method in ("POST", "PUT"):
            content_type = request.headers.get("content-type")
            if content_type and "application/json" in content_type:
                try:
                    body = await request.json()
                    for key, value in body.items():
                        if isinstance(value, str):
                            body[key] = bleach.clean(value)
                    # This is a hack to modify the request body
                    # A better approach would be to use a different middleware
                    # that allows modifying the request body.
                    scope = request.scope
                    async def receive():
                        return {"type": "http.request", "body": str(body).encode()}
                    request = Request(scope, receive)
                except Exception:
                    pass

        response = await call_next(request)
        return response
