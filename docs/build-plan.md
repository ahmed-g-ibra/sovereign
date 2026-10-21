# Sovereign — Build Plan

**What it is:** a decision tool for Gulf cloud architects that answers one question with evidence —
*where is this workload allowed to live, and what does that choice cost me in latency?*

Three surfaces:

1. **Latency matrix** — continuously measured network performance from GCC ISPs to every
   Middle East cloud region, with history.
2. **Residency knowledge base** — per-country data-protection and localisation rules, versioned,
   cited, dated.
3. **Advisor** — a short wizard (workload type, data classification, user countries) that returns
   permitted regions ranked by *your* measured latency, exportable as a decision record.

The latency data is the moat. Anyone can summarise a law; nobody else has 18 months of
measurements from Omantel and stc to me-central-1.

---

## 0. Timing — why now

As of October 2026:

| Provider | Live in GCC | Imminent |
|---|---|---|
| AWS | Bahrain (me-south-1), UAE (me-central-1) | **Saudi Arabia — targeted December 2026**, 3 AZs |
| Azure | UAE North/Central, Qatar Central | **Saudi Arabia East — Q4 2026**, 3 AZs |
| Google | Dammam, Doha | Kuwait (announced, no date) |
| Oracle | Jeddah, Riyadh, Abu Dhabi, Dubai, Muscat | — |

Two major Saudi regions land within weeks. If probes are live *before* launch day, you own the
first independent latency numbers for both — day-one benchmarks nobody can retroactively produce.
That is the launch moment for this project. Verify each region's status against the provider's own
locations page before publishing; these dates are targets, not guarantees.

**Implication for the roadmap:** Phase 1 is time-sensitive. Get probes running against the existing
five regions now, so the Saudi regions are just two new rows in a system that already works.

---

## 1. Measurement design

This is the part that must be right. Everything else is a normal web app.

### What we measure

| Metric | Method | Interval |
|---|---|---|
| TCP connect time | TCP handshake to port 443 of the regional endpoint | 60s |
| TLS handshake | full handshake, separately timed | 60s |
| HTTP TTFB | GET against our own in-region responder | 60s |
| Packet loss / jitter | ICMP burst where permitted, TCP-ping fallback | 60s |
| Path / AS hops | traceroute, recorded with AS numbers | hourly |
| DNS resolution | time to resolve the regional endpoint via local resolver | 5 min |

Report **p50, p90, p95, p99 and loss** over rolling windows. Never publish a bare average — the
tail is the whole story for anyone choosing a region.

### What we measure *against*

- **Phase 1:** each provider's existing public regional endpoints (e.g. the regional S3 and
  Cloud Storage endpoints). Zero cost, available immediately, slightly less controlled.
- **Phase 2:** our own tiny responder deployed into every target region by Terraform — a container
  returning a timestamp and nothing else. Apples-to-apples across providers, no reliance on vendor
  endpoint behaviour, and no hammering of someone else's service. This is the honest way to do it
  and it is also the part that proves multi-cloud IaC skill.

### Vantage points

Cloud-to-cloud numbers are easy and nearly worthless — everyone's backbone looks good. The value is
**last-mile**: what a real user or a real office connection sees.

| Tier | What | Why |
|---|---|---|
| A | Your own device at home on Omantel, and a second on Ooredoo | authentic Omani last mile, free |
| B | Cheap VPS in UAE, Bahrain, Saudi, Qatar, Kuwait | stable baselines, always on |
| C | One probe inside each cloud region | region-to-region matrix, isolates last-mile effects |
| D | Community probes — `docker run` one-liner, anyone can host | ISP diversity at zero cost to you |
| E | Browser RUM — a "measure from your connection" button on the site | wide coverage, free, noisy |

Tiers A–C are enough to launch. D and E are how this scales past what you can pay for, and they are
the growth loop: contributors check back to see their own ISP on the chart.

### Probe agent

- **Single static Go binary.** Cross-compiles to ARM for a Raspberry Pi, runs in ~15 MB, no runtime
  to install on a stranger's machine. This is the one place where Go is clearly the right call.
- Pulls its schedule from the control plane, so you can change what is measured without redeploying
  probes.
- **Buffers to local disk when offline** and replays on reconnect — otherwise an ISP outage silently
  becomes missing data instead of the most interesting data you have.
- Registers once, gets an identity and token; heartbeats every 60s.
- Randomised jitter on every schedule so 200 probes don't fire simultaneously.
- Self-reports host health (CPU load, clock skew). **A loaded probe host produces fake latency** —
  you must be able to discard its data after the fact.

### Measurement hygiene (the credibility rules)

- A silent probe is *unknown*, never *down*. Distinguish the two everywhere.
- Clock skew > 500 ms → quarantine that probe's data.
- Publish the methodology page before publishing the numbers.
- Exclude probes with fewer than N samples in the window from any published aggregate.
- Never present a single probe as representing a country.

---

## 2. Architecture

```
  Probes (Go agent, tiers A–E)
        │  signed HTTPS batch POST every 60s
        ▼
  ALB ──► ECS Fargate: ingest + API (containers)
        │
        ▼
  RDS PostgreSQL (raw samples, partitioned by day)
        │
        ├── EventBridge ──► Lambda: rollup jobs (1m → 5m → 1h → 1d)
        ├── EventBridge ──► Lambda: residency re-verification reminders
        │
        ▼
  CloudFront + S3 ──► Astro static site (matrix, KB, advisor)
        │
  Route 53 ──► everything
```

### AWS services, and why each one

| Service | Role | Why not the alternative |
|---|---|---|
| **ECS Fargate** | ingest + API containers | Lambda would be cheaper, but containers + ALB is the skill you are building toward, and ingest is steady-state so there is nothing to scale to zero |
| **Lambda + EventBridge** | rollups, scheduled checks, re-verification | genuinely bursty, genuinely idle between runs — the right tool here |
| **RDS PostgreSQL** | samples + app data | one database for both, no second system to operate; Timestream and ClickHouse are premature at this volume |
| **S3 + CloudFront** | static site, dataset downloads | the site is static; there is no reason to serve it from a container |
| **Route 53** | DNS | also gives you health checks for free if you want them |
| **ECR** | container images | — |
| **Secrets Manager / SSM** | probe signing keys, DB credentials | Parameter Store for config, Secrets Manager only for actual secrets — cheaper |
| **IAM + GitHub OIDC** | CI/CD auth | **no long-lived AWS keys anywhere**, which is a specific thing worth being able to describe in an interview |
| **CloudWatch** | logs, alarms | Grafana Cloud free tier for the dashboards you want to look at |

### Data volume — do the math before choosing a database

```
30 probes × 12 endpoints × 1 sample/min
  = 360 samples/min = ~518k/day = ~16M/month
```

Postgres handles that comfortably with daily partitions and scheduled rollups. Keep **raw samples
30 days**, **5-minute aggregates 1 year**, **hourly aggregates forever**. Hourly data from 2026
is the asset; raw pings from last March are not.

Revisit the storage choice at 100M rows/month, not before. Reaching for ClickHouse on day one is
the classic portfolio-project mistake — it signals you cannot size a system.

---

## 3. Residency knowledge base — git as the CMS

Each country is a Markdown file with structured frontmatter:

```yaml
country: OM
regulator: Ministry of Transport, Communications and IT
instrument: Personal Data Protection Law (Royal Decree 6/2022)
effective: 2023-02-13
localisation_required: conditional
cross_border: permitted with consent / adequacy conditions
source_urls: [...]
last_verified: 2026-10-21
verified_by: ahmed
```

Why git rather than a database: every change is a reviewed PR with an audit trail, the content is
diffable, and you can render **"last verified on 21 Oct 2026"** on every page. That date is what
makes the site trustworthy — and a quarterly Lambda that opens a GitHub issue for any file whose
`last_verified` is older than 90 days is what keeps it honest.

Hard rules: cite the instrument, link the primary source, never paraphrase a legal requirement
without a citation, and carry a standing *informational, not legal advice* notice. Start with
Oman, UAE and Saudi only. Three countries done properly beats six done vaguely.

---

## 4. Advisor

A YAML ruleset, not code:

```yaml
- id: saudi-personal-data-default
  when:
    user_countries: [SA]
    data_classes: [personal]
  verdict: conditional
  requires: [transfer_risk_assessment]
  candidate_regions: [gcp-dammam, azure-sa-east, aws-sa]
  citation: sdaia-pdpl-art-29
```

Pipeline: inputs → rules produce `allowed` / `conditional` / `blocked` per region with a citation →
allowed regions ranked by measured p95 from the relevant countries → output.

The output is the feature that gets this shared in Slack channels: a **permalinked decision record**
listing inputs, verdict, citations and the latency evidence as of that date, exportable to Markdown
or PDF. Architects need exactly that artifact for design reviews, and every export carries your URL.

---

## 5. Public API

```
GET /v1/regions
GET /v1/latency?from=om-omantel&to=aws:me-central-1&window=30d&percentile=95
GET /v1/matrix?window=7d
GET /v1/residency/OM
GET /v1/advise           (POST the wizard inputs, get the decision record)
```

Open, rate-limited, documented with OpenAPI. Publish monthly aggregates as downloadable CSV/Parquet
under CC BY — citations from other people's blog posts are the cheapest marketing that exists.

---

## 6. Stack

| Layer | Choice | Reason |
|---|---|---|
| Probe agent | **Go** | static binary, ARM, tiny, no runtime on hosts you don't control |
| API / ingest | **Python + FastAPI** | you are still building coding fluency; velocity matters more than nanoseconds for a service doing 6 writes/sec |
| Frontend | **Astro** + a few React islands | content-heavy and mostly static; ships almost no JS |
| Charts | **ECharts** | handles dense time-series and heatmaps without a fight |
| DB | **PostgreSQL** (RDS) | one store, partitioned, boring on purpose |
| IaC | **Terraform** | multi-cloud is the entire point; CDK would lock you to AWS |
| CI/CD | **GitHub Actions** → OIDC → ECR → ECS | no static credentials |
| Monitoring | CloudWatch + Grafana Cloud free tier | watch your own system from outside itself |

If you would rather carry one language, make it Go everywhere — it costs you some early speed and
buys a cleaner story. Do not split three ways.

---

## 7. Repo layout

```
sovereign/
├── agent/            Go probe agent
├── api/              FastAPI service (ingest + public API)
├── web/              Astro site
├── infra/
│   ├── aws/          core platform
│   ├── responders/   per-region responder modules (aws/gcp/azure/oci)
│   └── modules/
├── knowledge/        residency Markdown + rules YAML
├── docs/             methodology, ADRs, runbooks
└── .github/workflows/
```

Keep an **ADR per real decision** in `docs/`. When an interviewer asks why Postgres over Timestream,
you hand them a dated document instead of improvising.

---

## 8. Roadmap

### Phase 0 — Hello, data (1–2 weeks)
Repo, domain, Terraform skeleton with remote state, CI/CD to a static page. One probe — your own
machine on Omantel — measuring TCP/TLS to the two live AWS regions, writing to Postgres. One ugly
chart.
**Exit:** real numbers from your own connection, in a database, deployed by pipeline.

### Phase 1 — Public matrix (weeks 3–6) ← *time-sensitive*
Go agent with registration, auth, buffering and self-health. Five to eight vantage points across
Oman, UAE, Bahrain, Saudi, Qatar. All live ME regions across the four providers. Rollups. Public
latency matrix and methodology page.
**Exit:** site is live and the Saudi region launches are a config change, not a project.

### Phase 2 — Credibility (weeks 7–12)
Own responders deployed per region via Terraform across all four clouds. Traceroute and AS-path
analysis. Residency KB for Oman, UAE, Saudi. Advisor v1 with decision-record export. Public API.
**Exit:** someone who is not you uses it to make a real decision.

### Phase 3 — Scale and reach (months 4–6)
Community probes with a one-line install. Browser RUM contribution. Remaining GCC countries.
Degradation alerting. **Monthly "State of Gulf Cloud Latency" report** — this is what gets you cited,
and it costs an afternoon once the data exists.
**Exit:** probes you did not install, and an inbound link you did not ask for.

### Phase 4 — Operate it like production (month 6+)
Multi-AZ, defined SLOs with error budgets, a public status page for Sovereign itself, cost
dashboard, game-day chaos exercises, incident write-ups published openly. Optional paid tier:
API quota, private probes inside a company's own network.
**Exit:** you can describe an incident you detected, diagnosed and fixed in your own production
system — the single most useful interview story available to someone moving out of support.

---

## 9. Running cost

| Phase | Monthly | Main lines |
|---|---|---|
| 0 | **~$5** | Route 53, S3/CloudFront, free-tier RDS |
| 1 | **~$30–45** | RDS t4g.micro, one Fargate task, ALB (~$18 — the biggest single line), 3–4 VPS |
| 2 | **~$70–100** | + per-region responders across four clouds (~$4 each), more probes |
| 3+ | **~$90–140** | + redundancy, more storage, egress |

Cost controls from day one: a Budgets alarm at $50, `t4g`/Graviton everywhere, S3 lifecycle rules on
raw data, and ECS Fargate Spot for anything that can tolerate interruption. Egress is the line item
that surprises people — measure it deliberately, and consider dropping the ALB for a single Fargate
task with a public IP in Phase 1 if $18/month matters.

---

## 10. Risks

| Risk | Mitigation |
|---|---|
| **Probe host bias** — a loaded VPS reports fake latency | measure host CPU/load alongside every sample; discard retroactively |
| **ICMP blocked on GCC ISPs** | TCP-ping as the primary method, ICMP as a bonus |
| **CGNAT distorts last-mile paths** | record observed public IP and AS; segment by AS, not by claimed ISP |
| **Legal content goes stale** | `last_verified` dates shown publicly + 90-day automated re-verification issues |
| **Looking like you are grading governments** | scope strictly to commercial cloud providers; no ministry or telecom rankings, no commentary |
| **Being mistaken for legal advice** | prominent standing disclaimer; the advisor outputs *candidates with citations*, never a verdict |
| **You get bored in month 4** | the monthly report is the forcing function — a public cadence you have to feed |
| **Probing seen as abusive** | your own responders by Phase 2, published methodology, conservative intervals, a contact address that works |

---

## 11. This week

1. Pick the name and register the domain.
2. `infra/aws` Terraform skeleton: remote state, OIDC role for GitHub Actions, a budget alarm.
3. Hard-code a list of five regional endpoints and write a 100-line Go program that times a TCP
   connect to each and prints the result. Run it on your laptop.
4. Stand up RDS free tier; write those samples to a table.
5. Leave it running on something that stays on at home. By Friday you have four days of data from
   an Omani connection that nobody else has.

Steps 1–5 are deliberately small. The project only works if the data starts accumulating before
the architecture is finished — every week you spend designing is a week of history you do not have.
