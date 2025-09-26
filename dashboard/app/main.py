from fastapi import FastAPI, Request
from fastapi.responses import HTMLResponse
from fastapi.templating import Jinja2Templates

from app.api import reports, servers, config, script, config_file, stats
from app.core.database import database
from app.core.settings import settings

from app.core.xss_middleware import XSSMiddleware

app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
)

app.add_middleware(XSSMiddleware)

templates = Jinja2Templates(directory="app/templates")

@app.on_event("startup")
async def startup():
    await database.connect()

@app.on_event("shutdown")
async def shutdown():
    await database.disconnect()

app.include_router(reports.router, prefix="/api/v1", tags=["reports"])
app.include_router(servers.router, prefix="/api/v1", tags=["servers"])
app.include_router(config.router, prefix="/api/v1", tags=["config"])
app.include_router(config_file.router, prefix="/api/v1", tags=["config_file"])
app.include_router(stats.router, prefix="/api/v1", tags=["stats"])
from app.core.websockets import manager

app.include_router(script.router, prefix="/api/v1", tags=["script"])

@app.websocket("/api/v1/ws")
async def websocket_endpoint(websocket: WebSocket, api_key: str):
    if api_key != settings.api_key:
        await websocket.close(code=1008)
        return
    await manager.connect(websocket)
    try:
        while True:
            await websocket.receive_text()
    except WebSocketDisconnect:
        manager.disconnect(websocket)

from fastapi.exceptions import HTTPException
from fastapi.responses import JSONResponse

@app.exception_handler(HTTPException)
async def http_exception_handler(request, exc):
    return JSONResponse(
        status_code=exc.status_code,
        content={"message": exc.detail},
    )

@app.get("/", response_class=HTMLResponse)
async def index(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})

@app.get("/config", response_class=HTMLResponse)
async def config_page(request: Request):
    return templates.TemplateResponse("config.html", {"request": request})

from app.core.csrf import get_csrf_token

@app.get("/api/v1/csrf-token")
async def csrf_token():
    return {"csrf_token": get_csrf_token()}

@app.get("/stats", response_class=HTMLResponse)
async def stats_page(request: Request):
    return templates.TemplateResponse("stats.html", {"request": request})