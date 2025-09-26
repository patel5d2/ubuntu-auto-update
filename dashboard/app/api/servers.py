from fastapi import APIRouter

from app.core.database import database, reports

router = APIRouter()

@router.get("/servers")
async def list_servers():
    query = reports.select().with_only_columns([reports.c.server]).distinct()
    return await database.fetch_all(query)
