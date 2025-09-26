import os
import json
import sqlite3
from datetime import datetime
from pathlib import Path
from typing import Optional

from fastapi import FastAPI, Request, HTTPException, Header
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.templating import Jinja2Templates

BASE_DIR = Path(__file__).resolve().parent
DB_PATH = BASE_DIR / "dashboard.db"
TEMPLATES = Jinja2Templates(directory=str(BASE_DIR / "templates"))

API_KEY = os.environ.get("DASHBOARD_API_KEY", "")

app = FastAPI(title="Ubuntu Auto-Update Dashboard", version="0.1.0")


def init_db():
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH)
    try:
        conn.execute(
            """
            CREATE TABLE IF NOT EXISTS reports (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                server TEXT NOT NULL,
                status TEXT NOT NULL,
                exit_code INTEGER NOT NULL,
                duration_seconds INTEGER NOT NULL,
                reboot_required INTEGER NOT NULL,
                timestamp TEXT NOT NULL,
                raw JSON NOT NULL,
                ip TEXT
            );
            """
        )
        conn.execute("CREATE INDEX IF NOT EXISTS idx_reports_server_ts ON reports(server, timestamp DESC);")
    finally:
        conn.commit()
        conn.close()


@app.on_event("startup")
async def on_startup():
    init_db()


def _require_api_key(header_key: Optional[str]):
    if not API_KEY:
        # If no API key configured server-side, reject to encourage secure config
        raise HTTPException(status_code=503, detail="Dashboard API key not configured on server")
    if not header_key or header_key != API_KEY:
        raise HTTPException(status_code=401, detail="Invalid or missing API key")


@app.post("/api/v1/reports")
async def ingest_report(request: Request, x_api_key: Optional[str] = Header(default=None, convert_underscores=False)):
    _require_api_key(x_api_key)
    try:
        data = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid JSON payload")

    required = ["server", "timestamp", "status", "exit_code", "duration_seconds", "reboot_required", "version"]
    missing = [k for k in required if k not in data]
    if missing:
        raise HTTPException(status_code=400, detail=f"Missing fields: {', '.join(missing)}")

    try:
        # Basic normalization/validation
        server = str(data["server"])[:255]
        status = str(data["status"])[:64]
        exit_code = int(data["exit_code"])
        duration = int(data["duration_seconds"])
        reboot_required = 1 if (data.get("reboot_required") in (True, 1, "true", "True")) else 0
        timestamp = str(data["timestamp"])[:64]
        raw = json.dumps(data, ensure_ascii=False)
        ip = request.client.host if request.client else None
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Validation error: {e}")

    conn = sqlite3.connect(DB_PATH)
    try:
        conn.execute(
            "INSERT INTO reports(server, status, exit_code, duration_seconds, reboot_required, timestamp, raw, ip) VALUES (?,?,?,?,?,?,?,?)",
            (server, status, exit_code, duration, reboot_required, timestamp, raw, ip),
        )
    finally:
        conn.commit()
        conn.close()

    return JSONResponse(status_code=201, content={"ok": True})


@app.get("/api/v1/reports")
async def list_reports(server: Optional[str] = None, limit: int = 100):
    limit = max(1, min(limit, 1000))
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    try:
        if server:
            rows = conn.execute(
                "SELECT * FROM reports WHERE server = ? ORDER BY datetime(timestamp) DESC, id DESC LIMIT ?",
                (server, limit),
            ).fetchall()
        else:
            rows = conn.execute(
                "SELECT * FROM reports ORDER BY datetime(timestamp) DESC, id DESC LIMIT ?",
                (limit,),
            ).fetchall()
        items = [dict(row) for row in rows]
    finally:
        conn.close()
    return {"items": items}


@app.get("/", response_class=HTMLResponse)
async def index(request: Request, server: Optional[str] = None):
    # Gather basic aggregates
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    try:
        servers = [r[0] for r in conn.execute("SELECT DISTINCT server FROM reports ORDER BY server ASC").fetchall()]
        if server and server not in servers:
            server = None
        if server:
            rows = conn.execute(
                "SELECT * FROM reports WHERE server = ? ORDER BY datetime(timestamp) DESC, id DESC LIMIT 100",
                (server,),
            ).fetchall()
        else:
            rows = conn.execute(
                "SELECT * FROM reports ORDER BY datetime(timestamp) DESC, id DESC LIMIT 100",
            ).fetchall()
        items = [dict(row) for row in rows]
    finally:
        conn.close()

    return TEMPLATES.TemplateResponse(
        "index.html",
        {"request": request, "servers": servers, "selected_server": server, "items": items},
    )
