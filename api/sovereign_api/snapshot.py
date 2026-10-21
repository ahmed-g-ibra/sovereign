"""Export everything the static site needs as JSON.

The site is static and the control plane runs on one laptop, so the build
cannot call the API. Instead the data is snapshotted into the web project and
the site renders what the snapshot contains, with the generation time shown on
every page.

That is an honest constraint rather than a workaround: a published figure
should say when it was measured, and a reader should be able to tell at a
glance whether they are looking at this morning or last month.
"""

from __future__ import annotations

import datetime as dt
import json
import pathlib
from typing import Any

from . import knowledge
from .licensing import DATA_LICENSE

# Short enough to be current, long enough that one laptop sleeping overnight
# does not empty the site.
DEFAULT_WINDOW = "7 days"
MIN_SAMPLES = 10


async def _rows(conn, sql: str, params: tuple = ()) -> list[dict]:
    cur = await conn.execute(sql, params)
    return [dict(r) for r in await cur.fetchall()]


async def build(conn, *, window: str = DEFAULT_WINDOW,
                min_samples: int = MIN_SAMPLES) -> dict[str, Any]:
    entries = knowledge.load_entries()
    rules = knowledge.load_rules()

    matrix = await _rows(conn, """
        SELECT s.target_id, t.provider, t.region, t.country AS target_country,
               t.city, t.is_control, p.country AS probe_country,
               count(*)::int AS samples,
               round(((count(*) FILTER (WHERE s.error IS NOT NULL))::numeric
                      / count(*) * 100), 2)::float8 AS loss_pct,
               round(percentile_cont(0.50) WITHIN GROUP (ORDER BY s.total_ms)
                     FILTER (WHERE s.error IS NULL)::numeric, 1)::float8 AS p50_ms,
               round(percentile_cont(0.95) WITHIN GROUP (ORDER BY s.total_ms)
                     FILTER (WHERE s.error IS NULL)::numeric, 1)::float8 AS p95_ms,
               round(avg(s.dns_ms) FILTER (WHERE s.error IS NULL)::numeric, 1)::float8 AS dns_ms,
               round(avg(s.connect_ms) FILTER (WHERE s.error IS NULL)::numeric, 1)::float8 AS connect_ms
        FROM samples s
        JOIN probes p ON p.id = s.probe_id
        JOIN targets t ON t.id = s.target_id
        WHERE s.ts > now() - %s::interval
          AND p.quarantined IS FALSE AND p.revoked_at IS NULL
        GROUP BY s.target_id, t.provider, t.region, t.country, t.city,
                 t.is_control, p.country
        HAVING count(*) >= %s
        ORDER BY p50_ms NULLS LAST
    """, (window, min_samples))

    # One trace per probe and target: the most recent.
    paths = await _rows(conn, """
        SELECT DISTINCT ON (t.probe_id, t.target_id)
               t.target_id, tg.provider, tg.region, tg.country AS target_country,
               t.ts, host(t.dest_ip) AS dest_ip, t.complete, t.hop_count,
               t.answered, t.as_path, p.country AS probe_country,
               coalesce((SELECT json_agg(json_build_object(
                    'ttl', h.ttl, 'ip', host(h.hop_ip), 'rtt_ms', h.rtt_ms,
                    'asn', h.asn, 'as_name', h.as_name, 'final', h.final)
                    ORDER BY h.ttl)
                 FROM traceroute_hops h WHERE h.traceroute_id = t.id),
                 '[]'::json) AS hops
        FROM traceroutes t
        JOIN probes  p  ON p.id = t.probe_id
        JOIN targets tg ON tg.id = t.target_id
        WHERE p.quarantined IS FALSE AND p.revoked_at IS NULL
        ORDER BY t.probe_id, t.target_id, t.ts DESC
    """)

    changes = await _rows(conn, """
        SELECT c.target_id, c.ts, c.previous_ts, c.as_path, c.previous_path,
               p.country AS probe_country, t.provider, t.region
        FROM path_changes c
        JOIN probes  p ON p.id = c.probe_id
        JOIN targets t ON t.id = c.target_id
        ORDER BY c.ts DESC
        LIMIT 100
    """)

    networks = await _rows(conn, """
        SELECT h.asn, max(h.as_name) AS as_name, count(*)::int AS hops_seen
        FROM traceroute_hops h
        WHERE h.asn IS NOT NULL
        GROUP BY h.asn ORDER BY hops_seen DESC
    """)

    targets = await _rows(conn, """
        SELECT id, provider, region, country, city, host, status, is_control, note
        FROM targets ORDER BY provider, id
    """)

    probes = await _rows(conn, """
        SELECT name, country, city, asn, tier, enrolled_at, last_seen_at,
               quarantined,
               (last_seen_at > now() - interval '10 minutes') AS reporting
        FROM probes WHERE revoked_at IS NULL ORDER BY country, name
    """)

    stats = (await _rows(conn, """
        SELECT count(*)::int AS samples,
               min(ts) AS oldest, max(ts) AS newest,
               (SELECT count(*)::int FROM traceroutes) AS traces,
               (SELECT count(*)::int FROM probes WHERE revoked_at IS NULL) AS probes
        FROM samples
    """))[0]

    return {
        "meta": {
            "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "window": window,
            "min_samples": min_samples,
            "kb_status": knowledge.overall_status(entries),
            "license": DATA_LICENSE,
            "stats": stats,
        },
        "matrix": matrix,
        "paths": paths,
        "changes": changes,
        "networks": networks,
        "targets": targets,
        "probes": probes,
        "residency": {
            code: {**e.public(), "body": e.body}
            for code, e in sorted(entries.items())
        },
        "rules": {
            "version": rules.version,
            "obligations": rules.obligations,
            "rules": rules.rules,
        },
    }


def write(data: dict[str, Any], out_dir: pathlib.Path) -> list[pathlib.Path]:
    out_dir.mkdir(parents=True, exist_ok=True)
    written = []
    for name, payload in data.items():
        path = out_dir / f"{name}.json"
        path.write_text(json.dumps(payload, indent=2, default=str) + "\n")
        written.append(path)
    return written
