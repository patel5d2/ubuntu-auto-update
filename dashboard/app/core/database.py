import databases
import sqlalchemy

from .settings import settings

database = databases.Database(settings.database_url)

metadata = sqlalchemy.MetaData()

reports = sqlalchemy.Table(
    "reports",
    metadata,
    sqlalchemy.Column("id", sqlalchemy.Integer, primary_key=True),
    sqlalchemy.Column("server", sqlalchemy.String),
    sqlalchemy.Column("status", sqlalchemy.String),
    sqlalchemy.Column("exit_code", sqlalchemy.Integer),
    sqlalchemy.Column("duration_seconds", sqlalchemy.Integer),
    sqlalchemy.Column("reboot_required", sqlalchemy.Boolean),
    sqlalchemy.Column("timestamp", sqlalchemy.String),
    sqlalchemy.Column("raw", sqlalchemy.JSON),
    sqlalchemy.Column("ip", sqlalchemy.String),
)

engine = sqlalchemy.create_engine(
    settings.database_url, connect_args={"check_same_thread": False}
)

metadata.create_all(engine)
