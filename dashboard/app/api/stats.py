from fastapi import APIRouter, Depends
from sqlalchemy import func, select

from app.core.database import database, reports
from app.core.security import get_api_key

router = APIRouter()

@router.get("/stats", dependencies=[Depends(get_api_key)])
async def get_stats():
    # Group by server and status
    query = (
        select(reports.c.server, reports.c.status, func.count().label("count"))
        .group_by(reports.c.server, reports.c.status)
    )
    server_status = await database.fetch_all(query)

    # Group by date and status
    query = (
        select(func.date(reports.c.timestamp).label("date"), reports.c.status, func.count().label("count"))
        .group_by(func.date(reports.c.timestamp), reports.c.status)
        .order_by(func.date(reports.c.timestamp))
    )
    date_status = await database.fetch_all(query)

    return {
        "server_status": server_status,
        "date_status": date_status,
    }
