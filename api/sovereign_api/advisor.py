"""The advisor: which regions may hold this workload, ranked by measured latency.

Deliberately not a verdict machine. It returns candidate regions with the
obligations attached to each and a citation for every claim, so that a human
can check the reasoning. The moment it starts sounding like legal advice it
becomes worse than useless, because people will believe it.
"""

from __future__ import annotations

import datetime as dt
from typing import Any

from .knowledge import Entry, Rules, overall_status

DATA_CLASSES = ("personal", "sensitive", "health", "financial", "government",
                "operational")

# Worst first. A region's verdict is the worst thing any jurisdiction says
# about it.
VERDICT_ORDER = {"blocked": 0, "unknown": 1, "conditional": 2, "allowed": 3}

DISCLAIMER = (
    "Informational only. This is not legal advice, and it is not a substitute "
    "for advice from a qualified practitioner in the relevant jurisdiction. "
    "Every claim links to the entry it came from; check the primary source "
    "before relying on it."
)


def _rule_applies(rule: dict[str, Any], country: str,
                  data_classes: list[str]) -> bool:
    countries = rule.get("countries", [])
    if "*" not in countries and country not in countries:
        return False
    classes = rule.get("data_classes", [])
    if "*" in classes:
        return True
    return any(c in classes for c in data_classes)


def advise(*, user_countries: list[str], data_classes: list[str],
           entries: dict[str, Entry], rules: Rules,
           candidate_regions: list[dict[str, Any]],
           latency: dict[str, dict[str, Any]],
           window: str, probe_countries: list[str]) -> dict[str, Any]:
    user_countries = [c.upper() for c in user_countries]
    warnings: list[str] = []

    # ---- jurisdictions -----------------------------------------------------
    jurisdictions = []
    applicable: list[dict[str, Any]] = []
    localisation: set[str] | None = None

    for country in user_countries:
        entry = entries.get(country)
        if entry is None:
            warnings.append(
                f"No residency entry for {country}; it was ignored rather than guessed at.")
            continue

        matched = [r for r in rules.rules
                   if _rule_applies(r, country, data_classes)]
        applicable.extend(matched)

        obligations = []
        for rule in matched:
            for key in rule.get("obligations") or []:
                spec = rules.obligations.get(key, {})
                obligations.append({
                    "key": key,
                    "label": spec.get("label", key),
                    "detail": spec.get("detail", ""),
                })

        for rule in matched:
            if rule.get("verdict") == "localised" and rule.get("requires_region_in"):
                allowed_here = set(rule["requires_region_in"])
                # Two localisation rules intersect: data must satisfy both.
                localisation = allowed_here if localisation is None \
                    else localisation & allowed_here

        worst = min((VERDICT_ORDER.get(_map_verdict(r.get("verdict")), 3)
                     for r in matched), default=3)
        jurisdictions.append({
            "country": country,
            "country_name": entry.country_name,
            "kb_status": entry.status,
            "regulator": entry.meta.get("regulator"),
            "instrument": entry.meta.get("instrument"),
            "verdict": _verdict_name(worst),
            "obligations": _dedupe(obligations),
            "notes": [r["note"].strip() for r in matched if r.get("note")],
            "open_questions": entry.open_questions,
            "citation": f"knowledge/residency/{country}.md",
            "primary_source": entry.meta.get("primary_source"),
        })

        if entry.status == "draft":
            warnings.append(
                f"{entry.country_name} is a DRAFT entry written from secondary "
                f"sources and not checked against the instrument.")
        if entry.is_stale:
            warnings.append(
                f"{entry.country_name} was last verified "
                f"{entry.days_since_verified} days ago.")

    if localisation is not None and not localisation:
        warnings.append(
            "Two localisation rules conflict: no region can satisfy both. "
            "This workload may not be lawfully deployable as specified.")

    # ---- regions -----------------------------------------------------------
    worst_rule_verdict = min(
        (VERDICT_ORDER.get(_map_verdict(r.get("verdict")), 3) for r in applicable),
        default=3)

    regions = []
    for region in candidate_regions:
        reasons: list[str] = []
        verdict = _verdict_name(worst_rule_verdict)

        if localisation is not None:
            if region["country"] in localisation:
                reasons.append(
                    f"Satisfies the localisation requirement ({'/'.join(sorted(localisation))}).")
            else:
                verdict = "blocked"
                reasons.append(
                    f"Localisation requires a region in {'/'.join(sorted(localisation))}; "
                    f"this region is in {region['country']}.")

        if verdict == "conditional" and not reasons:
            reasons.append("Permitted if the listed obligations are met.")
        if verdict == "unknown":
            reasons.append(
                "At least one jurisdiction's position could not be established. "
                "Treated as unknown rather than assumed permissive.")
        if verdict == "allowed" and not reasons:
            reasons.append("No obligation found in the rules for this combination.")

        measured = latency.get(region["id"])
        regions.append({
            "target_id": region["id"],
            "provider": region["provider"],
            "region": region["region"],
            "country": region["country"],
            "city": region.get("city"),
            "verdict": verdict,
            "reasons": reasons,
            "latency": measured,
        })

    def rank(r: dict[str, Any]) -> tuple[int, float, str]:
        # A target can have samples and still have no percentile: if every
        # probe failed, p95 is null and loss is 100%. Those sort last among
        # their verdict, not first.
        p95 = (r["latency"] or {}).get("p95_ms")
        return (
            VERDICT_ORDER.get(r["verdict"], 9) * -1,   # allowed first
            p95 if p95 is not None else float("inf"),  # then fastest
            r["target_id"],
        )

    regions.sort(key=rank)

    measured_count = sum(1 for r in regions if r["latency"])
    if measured_count == 0:
        warnings.append(
            "No latency measurements in this window, so the ranking is by "
            "verdict only.")

    return {
        "generated_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        "status": overall_status(entries),
        "warnings": warnings,
        "inputs": {
            "user_countries": user_countries,
            "data_classes": data_classes,
        },
        "jurisdictions": jurisdictions,
        "regions": regions,
        "evidence": {
            "window": window,
            "probe_countries": probe_countries,
            "regions_with_measurements": measured_count,
            "note": ("Percentiles over successful probes only, quarantined "
                     "probes excluded."),
        },
        "rules_version": rules.version,
        "disclaimer": DISCLAIMER,
    }


def _map_verdict(v: str | None) -> str:
    # A localisation rule is not itself a refusal: it constrains where, and the
    # per-region check applies it. Everything else maps straight through.
    return "conditional" if v == "localised" else (v or "allowed")


def _verdict_name(order: int) -> str:
    for name, value in VERDICT_ORDER.items():
        if value == order:
            return name
    return "allowed"


def _dedupe(obligations: list[dict[str, str]]) -> list[dict[str, str]]:
    seen, out = set(), []
    for o in obligations:
        if o["key"] not in seen:
            seen.add(o["key"])
            out.append(o)
    return out


def to_markdown(record: dict[str, Any], record_id: str | None = None) -> str:
    """Render a decision record for pasting into a design review."""
    lines = ["# Cloud region decision record", ""]
    if record_id:
        lines.append(f"**Record:** `{record_id}`  ")
    lines += [
        f"**Generated:** {record['generated_at']}  ",
        f"**Knowledge base status:** {record['status'].upper()}  ",
        f"**Rules version:** {record['rules_version']}",
        "",
        "## Inputs",
        "",
        f"- Users in: {', '.join(record['inputs']['user_countries']) or 'none specified'}",
        f"- Data classes: {', '.join(record['inputs']['data_classes'])}",
        "",
    ]

    if record["warnings"]:
        lines += ["## Warnings", ""]
        lines += [f"- {w}" for w in record["warnings"]]
        lines.append("")

    lines += ["## Jurisdictions", ""]
    for j in record["jurisdictions"]:
        lines += [
            f"### {j['country_name']} ({j['country']}) — {j['verdict']}",
            "",
            f"- Instrument: {j['instrument']}",
            f"- Regulator: {j['regulator']}",
            f"- Entry status: `{j['kb_status']}` — `{j['citation']}`",
        ]
        if j["obligations"]:
            lines.append("- Obligations:")
            lines += [f"    - **{o['label']}** — {o['detail']}" for o in j["obligations"]]
        if j["open_questions"]:
            lines.append("- Open questions:")
            lines += [f"    - {q}" for q in j["open_questions"]]
        lines.append("")

    lines += ["## Candidate regions", "",
              "| Region | Provider | Country | Verdict | p50 | p95 | Samples |",
              "|---|---|---|---|---|---|---|"]
    for r in record["regions"]:
        lat = r["latency"] or {}
        lines.append(
            f"| {r['region']} | {r['provider']} | {r['country']} | {r['verdict']} | "
            f"{lat.get('p50_ms', '—')} | {lat.get('p95_ms', '—')} | "
            f"{lat.get('samples', '—')} |")
    lines += ["",
              f"Latency measured over {record['evidence']['window']} from probes in "
              f"{', '.join(record['evidence']['probe_countries']) or 'any country'}. "
              f"{record['evidence']['note']}",
              "",
              "---", "", record["disclaimer"], ""]
    return "\n".join(lines)
