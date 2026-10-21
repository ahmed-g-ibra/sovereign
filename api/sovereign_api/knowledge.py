"""Residency knowledge base: markdown files with YAML frontmatter, in git.

Not a database table. Every change to a legal claim should be a reviewed diff
with a date on it, and `git log` should be able to answer "when did we start
saying that, and who approved it".
"""

from __future__ import annotations

import datetime as dt
import pathlib
from dataclasses import dataclass, field
from typing import Any

import yaml

KNOWLEDGE_DIR = pathlib.Path(__file__).resolve().parent.parent.parent / "knowledge"
RESIDENCY_DIR = KNOWLEDGE_DIR / "residency"
RULES_PATH = KNOWLEDGE_DIR / "rules" / "residency.yaml"

STATUSES = ("draft", "reviewed", "verified")
REQUIRED_KEYS = ("country", "country_name", "status", "regulator", "instrument")

# How long a reviewed entry stays trustworthy before someone must look again.
STALE_AFTER_DAYS = 90


@dataclass
class Entry:
    country: str
    country_name: str
    status: str
    meta: dict[str, Any]
    body: str
    path: pathlib.Path

    @property
    def last_verified(self) -> dt.date | None:
        v = self.meta.get("last_verified")
        if isinstance(v, dt.datetime):
            return v.date()
        return v if isinstance(v, dt.date) else None

    @property
    def open_questions(self) -> list[str]:
        return list(self.meta.get("open_questions") or [])

    @property
    def days_since_verified(self) -> int | None:
        if self.last_verified is None:
            return None
        return (dt.date.today() - self.last_verified).days

    @property
    def is_stale(self) -> bool:
        """Draft entries are not stale, they are unverified -- a different problem."""
        if self.status == "draft":
            return False
        days = self.days_since_verified
        return days is None or days > STALE_AFTER_DAYS

    def public(self) -> dict[str, Any]:
        out = {
            "country": self.country,
            "country_name": self.country_name,
            "status": self.status,
            "regulator": self.meta.get("regulator"),
            "instrument": self.meta.get("instrument"),
            "in_force": _isoformat(self.meta.get("in_force")),
            "localisation_required": self.meta.get("localisation_required"),
            "cross_border_transfer": self.meta.get("cross_border_transfer"),
            "primary_source": self.meta.get("primary_source"),
            "secondary_sources": self.meta.get("secondary_sources") or [],
            "last_verified": _isoformat(self.last_verified),
            "verified_by": self.meta.get("verified_by"),
            "stale": self.is_stale,
            "days_since_verified": self.days_since_verified,
            "open_questions": self.open_questions,
        }
        return out


@dataclass
class Rules:
    version: int
    updated_at: Any
    obligations: dict[str, dict[str, str]]
    rules: list[dict[str, Any]] = field(default_factory=list)


def _isoformat(v: Any) -> str | None:
    if isinstance(v, (dt.date, dt.datetime)):
        return v.isoformat()
    return v if isinstance(v, str) else None


def parse_frontmatter(text: str, path: pathlib.Path) -> tuple[dict[str, Any], str]:
    if not text.startswith("---"):
        raise ValueError(f"{path.name}: missing YAML frontmatter")
    parts = text.split("---", 2)
    if len(parts) < 3:
        raise ValueError(f"{path.name}: frontmatter is not terminated")
    meta = yaml.safe_load(parts[1]) or {}
    if not isinstance(meta, dict):
        raise ValueError(f"{path.name}: frontmatter is not a mapping")
    return meta, parts[2].strip()


def load_entries(directory: pathlib.Path | None = None) -> dict[str, Entry]:
    directory = directory or RESIDENCY_DIR
    entries: dict[str, Entry] = {}
    for path in sorted(directory.glob("*.md")):
        meta, body = parse_frontmatter(path.read_text(), path)
        entry = Entry(
            country=str(meta.get("country", "")).upper(),
            country_name=meta.get("country_name", ""),
            status=meta.get("status", "draft"),
            meta=meta,
            body=body,
            path=path,
        )
        entries[entry.country] = entry
    return entries


def load_rules(path: pathlib.Path | None = None) -> Rules:
    doc = yaml.safe_load((path or RULES_PATH).read_text())
    return Rules(
        version=doc["version"],
        updated_at=doc.get("updated_at"),
        obligations=doc.get("obligations", {}),
        rules=doc.get("rules", []),
    )


def check(entries: dict[str, Entry] | None = None,
          rules: Rules | None = None) -> list[str]:
    """Return every problem found. Empty list means the knowledge base is sound.

    Used by `sovereign knowledge check` and by a test, so a malformed entry
    fails CI rather than surfacing as a 500 on the advisor.
    """
    entries = entries if entries is not None else load_entries()
    rules = rules if rules is not None else load_rules()
    problems: list[str] = []

    if not entries:
        problems.append("no residency entries found")

    for code, e in sorted(entries.items()):
        name = e.path.name
        for key in REQUIRED_KEYS:
            if not e.meta.get(key):
                problems.append(f"{name}: missing required key '{key}'")
        if e.status not in STATUSES:
            problems.append(f"{name}: status '{e.status}' is not one of {STATUSES}")
        if len(code) != 2:
            problems.append(f"{name}: country '{code}' is not a two-letter code")
        if e.status != "draft":
            # Claiming a status above draft requires the evidence for it.
            if not e.last_verified:
                problems.append(f"{name}: status '{e.status}' requires last_verified")
            if not e.meta.get("verified_by"):
                problems.append(f"{name}: status '{e.status}' requires verified_by")
            if e.is_stale:
                problems.append(
                    f"{name}: verified {e.days_since_verified} days ago, "
                    f"over the {STALE_AFTER_DAYS}-day limit; re-verify or downgrade")
        if e.meta.get("open_questions") is not None and \
                not isinstance(e.meta["open_questions"], list):
            problems.append(f"{name}: open_questions must be a list")

    known_obligations = set(rules.obligations)
    for rule in rules.rules:
        rid = rule.get("id", "?")
        if not rule.get("countries"):
            problems.append(f"rule {rid}: no countries")
        if rule.get("verdict") not in ("allowed", "conditional", "localised", "unknown"):
            problems.append(f"rule {rid}: bad verdict '{rule.get('verdict')}'")
        for ob in rule.get("obligations") or []:
            if ob not in known_obligations:
                problems.append(f"rule {rid}: unknown obligation '{ob}'")
        citation = rule.get("citation")
        if citation and citation not in entries:
            problems.append(f"rule {rid}: cites '{citation}' with no such entry")
        for c in rule.get("countries", []):
            if c != "*" and c not in entries:
                problems.append(f"rule {rid}: country '{c}' has no residency entry")

    return problems


def overall_status(entries: dict[str, Entry]) -> str:
    """The weakest link. One draft entry makes the whole answer draft."""
    if any(e.status == "draft" for e in entries.values()):
        return "draft"
    if any(e.is_stale for e in entries.values()):
        return "stale"
    if all(e.status == "verified" for e in entries.values()):
        return "verified"
    return "reviewed"
