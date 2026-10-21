"""FastAPI application."""

from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI

from .db import make_pool
from .routes import agent, enrollment, public
from .settings import Settings


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings: Settings = app.state.settings
    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    app.state.pool = pool
    async with pool.connection() as conn:
        # Cheap, idempotent, and it means a deploy that happens to land at
        # 23:59 does not fail its first inserts a minute later.
        await conn.execute("SELECT ensure_sample_partitions(1, 7)")
    try:
        yield
    finally:
        await pool.close()


def create_app(settings: Settings | None = None) -> FastAPI:
    app = FastAPI(
        title="Sovereign",
        version="0.1.0",
        summary="Latency and data-residency intelligence for Gulf cloud regions",
        lifespan=lifespan,
    )
    app.state.settings = settings or Settings.from_env()
    app.include_router(public.router)
    app.include_router(enrollment.router)
    app.include_router(agent.router)
    return app
