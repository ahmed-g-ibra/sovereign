# Knowledge base

Residency rules as data, in git. Every change is a reviewed diff with a date
attached, and every page renders the date it was last verified.

## Status vocabulary

| `status` | Meaning | May the site publish it? |
|---|---|---|
| `draft` | Written from secondary sources. Not checked against the instrument. | Only with a visible DRAFT banner |
| `reviewed` | Someone read the primary instrument and confirmed each claim | Yes, with the verification date |
| `verified` | Reviewed, plus confirmed by a qualified practitioner in that jurisdiction | Yes |

**Every entry here is currently `draft`.** They were written from law-firm
summaries and news reporting, not from the instruments themselves. The
machinery around them — validation, staleness, the advisor — is real and
tested. The legal content is a starting point for verification, not an answer.

## Verifying an entry

1. Open the primary instrument at the URL in `primary_source`.
2. Confirm each claim in the frontmatter against it.
3. Fill in `verified_by` and `last_verified`, set `status: reviewed`.
4. Anything you could not confirm moves to the `open_questions` list rather
   than being quietly dropped.

`sovereign knowledge check` fails on an entry that is stale, malformed, or
claims a status its metadata does not support.
