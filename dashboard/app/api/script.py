import subprocess

from fastapi import APIRouter, Depends, HTTPException

from app.core.security import get_api_key

router = APIRouter()

from app.core.csrf import validate_csrf_token

@router.post("/script/execute", dependencies=[Depends(get_api_key), Depends(validate_csrf_token)])
async def execute_script():
    try:
        subprocess.Popen(["/usr/local/bin/docker-entrypoint.sh", "update"])
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
    return {"ok": True}
