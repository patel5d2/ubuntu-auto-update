from typing import List

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel

from app.core.database import database, reports
from app.core.security import get_api_key

router = APIRouter()

class Report(BaseModel):
    server: str
    timestamp: str
    status: str
    exit_code: int
    duration_seconds: int
    reboot_required: bool
    version: str
    raw: dict
    ip: str

from app.core.websockets import manager

@router.post("/reports", dependencies=[Depends(get_api_key)])
async def ingest_report(report: Report):
    query = reports.insert().values(
        server=report.server,
        timestamp=report.timestamp,
        status=report.status,
        exit_code=report.exit_code,
        duration_seconds=report.duration_seconds,
        reboot_required=report.reboot_required,
        raw=report.raw,
        ip=report.ip,
    )
    await database.execute(query)
    await manager.broadcast(report.json())
    return {"ok": True}

@router.get("/reports")
async def list_reports(server: str = None, limit: int = 100):
    query = reports.select().order_by(reports.c.timestamp.desc()).limit(limit)
    if server:
        query = query.where(reports.c.server == server)
@router.delete("/reports", dependencies=[Depends(get_api_key)])
async def clear_reports():
    query = reports.delete()
    await database.execute(query)
    return {"ok": True}
