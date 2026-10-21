from __future__ import annotations

import datetime as dt
import pathlib

import pytest

from sovereign_api import knowledge


def test_every_entry_parses_and_validates():
    entries = knowledge.load_entries()
    assert entries, "no residency entries loaded"
    problems = knowledge.check(entries)
    assert problems == [], "knowledge base has structural problems:\n" + "\n".join(problems)


def test_gcc_countries_are_covered():
    entries = knowledge.load_entries()
    assert {"OM", "AE", "SA", "QA", "BH", "KW"} <= set(entries)


def test_entries_claiming_verification_must_show_their_work(tmp_path):
    """The invariant that keeps this honest: you cannot mark an entry reviewed
    without saying who reviewed it and when."""
    (tmp_path / "XX.md").write_text(
        "---\n"
        "country: XX\ncountry_name: Testland\nstatus: reviewed\n"
        "regulator: Someone\ninstrument: A Law\n"
        "---\nbody\n")
    entries = knowledge.load_entries(tmp_path)
    problems = knowledge.check(entries, knowledge.load_rules())
    assert any("last_verified" in p for p in problems)
    assert any("verified_by" in p for p in problems)


def test_stale_reviewed_entry_is_flagged(tmp_path):
    old = dt.date.today() - dt.timedelta(days=knowledge.STALE_AFTER_DAYS + 1)
    (tmp_path / "XX.md").write_text(
        f"---\n"
        f"country: XX\ncountry_name: Testland\nstatus: reviewed\n"
        f"regulator: Someone\ninstrument: A Law\n"
        f"last_verified: {old.isoformat()}\nverified_by: someone\n"
        f"---\nbody\n")
    entries = knowledge.load_entries(tmp_path)
    assert entries["XX"].is_stale
    assert any("re-verify" in p for p in knowledge.check(entries, knowledge.load_rules()))


def test_draft_entries_are_unverified_not_stale(tmp_path):
    """A draft is not stale -- it was never fresh. Different problem, different word."""
    (tmp_path / "XX.md").write_text(
        "---\ncountry: XX\ncountry_name: Testland\nstatus: draft\n"
        "regulator: Someone\ninstrument: A Law\n---\nbody\n")
    entries = knowledge.load_entries(tmp_path)
    assert entries["XX"].is_stale is False
    # Only entry-level problems matter here; the real rules file cites the real
    # countries, which this temp directory deliberately does not contain.
    problems = [p for p in knowledge.check(entries, knowledge.load_rules())
                if "XX.md" in p]
    assert problems == []


def test_malformed_frontmatter_is_rejected(tmp_path):
    (tmp_path / "XX.md").write_text("no frontmatter here\n")
    with pytest.raises(ValueError, match="frontmatter"):
        knowledge.load_entries(tmp_path)


def test_rules_cite_entries_that_exist():
    rules = knowledge.load_rules()
    entries = knowledge.load_entries()
    for rule in rules.rules:
        citation = rule.get("citation")
        if citation:
            assert citation in entries, f"rule {rule['id']} cites a missing entry"


def test_overall_status_is_the_weakest_link():
    entries = knowledge.load_entries()
    assert knowledge.overall_status(entries) == "draft", (
        "every entry is currently draft; if this fails, someone verified one "
        "and should confirm the advisor's warnings still make sense")
