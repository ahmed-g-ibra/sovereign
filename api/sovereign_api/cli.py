"""Operator CLI: sovereign <command>."""

from __future__ import annotations

import argparse
import asyncio
import datetime as dt
import secrets
import sys

from . import signing
from .db import make_pool, migrate
from .settings import Settings


async def cmd_migrate(settings: Settings, args) -> int:
    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    try:
        async with pool.connection() as conn:
            ran = await migrate(conn)
    finally:
        await pool.close()
    if ran:
        print("applied:", ", ".join(ran))
    else:
        print("already up to date")
    return 0


async def cmd_issue_token(settings: Settings, args) -> int:
    token = secrets.token_urlsafe(32)
    expires = dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=args.hours)

    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    try:
        async with pool.connection() as conn:
            await conn.execute(
                """
                INSERT INTO enrollment_tokens (token_hash, note, max_uses, expires_at)
                VALUES (%s, %s, %s, %s)
                """,
                (signing.hash_token(token), args.note, args.uses, expires),
            )
    finally:
        await pool.close()

    # Printed once. Only its hash is stored, so a lost token is reissued,
    # never recovered.
    print(token)
    print(f"# {args.uses} use(s), expires {expires.isoformat()}", file=sys.stderr)
    return 0


async def cmd_rollup(settings: Settings, args) -> int:
    now = dt.datetime.now(dt.timezone.utc)
    start = now - dt.timedelta(minutes=args.lookback)
    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    try:
        async with pool.connection() as conn:
            for resolution in ("5 minutes", "1 hour"):
                cur = await conn.execute(
                    "SELECT roll_up(%s::interval, %s, %s) AS n",
                    (resolution, start, now),
                )
                row = await cur.fetchone()
                print(f"{resolution}: {row['n']} buckets")
    finally:
        await pool.close()
    return 0


async def cmd_partitions(settings: Settings, args) -> int:
    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    try:
        async with pool.connection() as conn:
            cur = await conn.execute("SELECT ensure_sample_partitions(1, %s) AS n",
                                     (args.ahead,))
            print(f"created {(await cur.fetchone())['n']} partitions")
            if args.drop_older_than:
                cur = await conn.execute("SELECT drop_old_sample_partitions(%s) AS n",
                                         (args.drop_older_than,))
                print(f"dropped {(await cur.fetchone())['n']} partitions")
    finally:
        await pool.close()
    return 0


async def cmd_snapshot(settings: Settings, args) -> int:
    """Export the data the static site builds against."""
    import pathlib

    from . import snapshot

    out = pathlib.Path(args.out)
    pool = make_pool(settings.database_url)
    await pool.open(wait=True, timeout=30)
    try:
        async with pool.connection() as conn:
            data = await snapshot.build(conn, window=args.window,
                                        min_samples=args.min_samples)
    finally:
        await pool.close()

    written = snapshot.write(data, out)
    stats = data["meta"]["stats"]
    print(f"wrote {len(written)} files to {out}")
    print(f"  {stats['samples']} samples, {stats['traces']} traces, "
          f"{stats['probes']} probe(s)")
    print(f"  matrix rows: {len(data['matrix'])}, paths: {len(data['paths'])}, "
          f"path changes: {len(data['changes'])}")
    if not data["matrix"]:
        print("  NOTE: no matrix rows -- not enough samples in the window yet")
    return 0


async def cmd_knowledge(settings: Settings, args) -> int:
    """Validate the knowledge base. Runs in CI, so a malformed entry fails the
    build rather than surfacing as a 500 on the advisor."""
    from . import knowledge
    entries = knowledge.load_entries()
    problems = knowledge.check(entries)

    drafts = [e.country for e in entries.values() if e.status == "draft"]
    print(f"{len(entries)} entries, overall status: {knowledge.overall_status(entries)}")
    if drafts:
        print(f"draft (unverified): {', '.join(sorted(drafts))}")
    for e in sorted(entries.values(), key=lambda x: x.country):
        if e.open_questions:
            print(f"  {e.country}: {len(e.open_questions)} open question(s)")

    if problems:
        print("\nPROBLEMS:")
        for p in problems:
            print(f"  - {p}")
        return 1
    print("\nno structural problems")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="sovereign")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("migrate", help="apply pending migrations")

    p = sub.add_parser("issue-token", help="mint an enrolment token")
    p.add_argument("--uses", type=int, default=1)
    p.add_argument("--hours", type=int, default=24)
    p.add_argument("--note", default=None)

    p = sub.add_parser("rollup", help="recompute rollups")
    p.add_argument("--lookback", type=int, default=30, help="minutes")

    sp = sub.add_parser("snapshot", help="export JSON for the static site")
    sp.add_argument("--out", default="../web/src/data")
    sp.add_argument("--window", default="7 days")
    sp.add_argument("--min-samples", type=int, default=10)

    kp = sub.add_parser("knowledge", help="validate the residency knowledge base")
    kp.add_argument("action", nargs="?", default="check", choices=["check"])

    p = sub.add_parser("partitions", help="create and retire sample partitions")
    p.add_argument("--ahead", type=int, default=7)
    p.add_argument("--drop-older-than", type=int, default=None, help="days")

    args = parser.parse_args(argv)
    # Only commands that touch the database need settings; validating the
    # knowledge base is a pure file operation and must not require one.
    settings = None if args.command == "knowledge" else Settings.from_env()
    handler = {
        "migrate": cmd_migrate,
        "issue-token": cmd_issue_token,
        "rollup": cmd_rollup,
        "partitions": cmd_partitions,
        "knowledge": cmd_knowledge,
        "snapshot": cmd_snapshot,
    }[args.command]
    return asyncio.run(handler(settings, args))


if __name__ == "__main__":
    raise SystemExit(main())
