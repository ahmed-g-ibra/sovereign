"""Database pool and migration runner."""

from __future__ import annotations

import pathlib

from psycopg import AsyncConnection
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

MIGRATIONS_DIR = pathlib.Path(__file__).resolve().parent.parent / "migrations"


def make_pool(database_url: str, *, open_pool: bool = False) -> AsyncConnectionPool:
    return AsyncConnectionPool(
        database_url,
        min_size=1,
        max_size=10,
        open=open_pool,
        kwargs={"row_factory": dict_row},
    )


async def migrate(conn: AsyncConnection) -> list[str]:
    """Apply every unapplied migration in filename order.

    Deliberately not Alembic. The schema is a handful of tables, the project
    has one deployment, and a plain SQL file is readable by anyone who knows
    Postgres without also knowing a Python migration DSL.
    """
    await conn.execute(
        """
        CREATE TABLE IF NOT EXISTS schema_migrations (
            filename   text PRIMARY KEY,
            applied_at timestamptz NOT NULL DEFAULT now()
        )
        """
    )
    await conn.commit()

    cur = await conn.execute("SELECT filename FROM schema_migrations")
    applied = {row["filename"] for row in await cur.fetchall()}

    ran: list[str] = []
    for path in sorted(MIGRATIONS_DIR.glob("*.sql")):
        if path.name in applied:
            continue
        # Each migration runs in its own transaction: a failure halfway leaves
        # the database on the last good version rather than somewhere new.
        async with conn.transaction():
            await conn.execute(path.read_text())
            await conn.execute(
                "INSERT INTO schema_migrations (filename) VALUES (%s)", (path.name,)
            )
        ran.append(path.name)
    return ran
