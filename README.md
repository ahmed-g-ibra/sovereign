# Sovereign

Independent latency and data-residency intelligence for Gulf cloud regions.

> Where is this workload allowed to live, and what does that choice cost me in
> latency?

**Phase 0** is a Terraform skeleton, CI/CD with no long-lived AWS credentials,
a cost alarm, and a probe that measures real latency from a Gulf connection to
every Middle East cloud region.

**The site** ([`web/`](web/)) is a static Astro build: the latency matrix, AS
paths, residency pages and a region advisor that runs entirely in the browser.
No server, free to host, and it builds with or without data.

**Phase 2 (in progress)** adds the other half of the product: a residency
knowledge base in git and an advisor that answers *where may this workload
live, and what does that choice cost in latency* with a citable decision
record. No paid infrastructure is involved — see
[`docs/costs.md`](docs/costs.md) for what is deferred until there is budget,
and [`docs/running-free.md`](docs/running-free.md) for running the whole thing
on one machine at zero cost.

**Phase 1** turns that probe into a fleet: a control plane that decides what to
measure, Ed25519 probe enrolment, an ingest API with idempotent writes, disk
buffering that survives an outage, and Postgres with partitioned samples and
percentile rollups.

Full plan: [`docs/build-plan.md`](docs/build-plan.md)

## Why the clock matters

AWS is targeting **December 2026** for its first Saudi region and Azure's
**Saudi Arabia East** is slated for Q4 2026. Probes running before launch day
produce the first independent benchmarks for both — numbers that cannot be
generated retroactively. Everything in Phase 0 exists to make those launches a
one-line change to `agent/targets.json`.

## Layout

```
agent/      Go probe: measures latency and paths, buffers to disk, uploads
api/        FastAPI control plane + ingest, Postgres schema and migrations
web/        Astro static site, built from a JSON snapshot
knowledge/  residency entries, advisor rules, cross-language fixtures
infra/
  bootstrap/  S3 state bucket (local state, run once)
  aws/        GitHub OIDC roles + budget alarm (remote state)
docs/
  methodology.md   what is measured and the rules we hold ourselves to
  adr/             dated decisions with their alternatives
```

## The probe

No control plane yet. It prints to stdout, which is enough to start
accumulating history today.

```bash
cd agent

# Which endpoints actually resolve? Unverified guesses included.
go run ./cmd/probe -verify

# One round against confirmed-live regions.
go run ./cmd/probe

# Twenty rounds, ten seconds apart, then a percentile summary.
go run ./cmd/probe -count 20 -interval 10s

# Run until interrupted, JSON lines to a file — this is the one that matters.
go run ./cmd/probe -count 0 -json >> ../data/$(date +%F).ndjson
```

Flags: `-targets`, `-filter`, `-count`, `-interval`, `-timeout`, `-json`,
`-verify`, `-all`.

`-filter` matches id, provider, region, country or status:

```bash
go run ./cmd/probe -filter aws          # AWS only
go run ./cmd/probe -filter oci,sa       # Oracle regions in Saudi Arabia
```

### Target status

| Status | Meaning |
|---|---|
| `live` | region is GA and the endpoint is confirmed; measured by default |
| `verify` | documented endpoint pattern, not yet confirmed; needs `-verify` or `-all` |
| `announced` | region not open yet; kept so launch day is a status flip |

## Running the fleet locally

```bash
# 1. Postgres on port 55432.
docker compose up -d db

# 2. Schema and seed data.
cd api
uv sync --extra dev
export DATABASE_URL=postgresql://sovereign:sovereign@localhost:55432/sovereign
uv run sovereign migrate

# 3. Control plane.
uv run uvicorn sovereign_api.app:create_app --factory --port 8000

# 4. Mint an enrolment token (prints once; only its hash is stored).
uv run sovereign issue-token --note "muscat home probe"

# 5. Enrol a probe and start measuring.
cd ../agent
go run ./cmd/probe -register -server http://127.0.0.1:8000 \
  -token "$TOKEN" -name muscat-home -country OM -city Muscat -isp Omantel -tier core
go run ./cmd/probe -server http://127.0.0.1:8000
```

The agent generates its own Ed25519 keypair and sends only the public half, so
the server never holds anything that could forge a measurement. Its identity
and spool live under `agent/state/` (mode 0600).

### Endpoints

| Method | Path | Auth |
|---|---|---|
| `POST` | `/v1/probes/register` | enrolment token |
| `GET` | `/v1/config` | probe signature |
| `POST` | `/v1/samples` | probe signature |
| `POST` | `/v1/traceroutes` | probe signature |
| `GET` | `/v1/paths`, `/v1/paths/changes` | public |
| `GET` | `/v1/matrix?window=24h` | public |
| `GET` | `/v1/residency`, `/v1/residency/{cc}` | public |
| `POST` | `/v1/advise` | public |
| `GET` | `/v1/advise/{id}`, `/v1/advise/{id}/markdown` | public |
| `GET` | `/v1/targets`, `/v1/probes`, `/healthz` | public |

### Path measurement

Every hour the agent traces the path to each target and annotates each hop with
the network announcing it, resolved through Team Cymru's DNS service. Hops are
stored for verification; change detection compares the ordered sequence of
autonomous systems, because hop addresses churn while networks do not.

```bash
curl -s 'localhost:8000/v1/paths?target_id=aws-me-central-1' | jq '.traces[0].as_path'
curl -s 'localhost:8000/v1/paths/changes?window=30d'
```

It needs no root: the agent uses a datagram ICMP socket, which is unprivileged
on macOS and needs only a sysctl group on Linux ([ADR 0008](docs/adr/0008-unprivileged-icmp-and-as-paths.md)).
A probe that cannot open one keeps measuring latency and says so once.

### The site

```bash
make site        # snapshot the database, then build to web/dist
make site-dev    # live reload on :4321
```

`web/dist` is a plain directory of HTML — deployable free to Cloudflare Pages,
GitHub Pages or anything else that serves files.

```bash
make deploy      # snapshot, build, upload to Cloudflare Pages
```

The site ships **unlisted**: a `noindex` meta tag, an `X-Robots-Tag` header and
a deliberately permissive `robots.txt`, all generated from one flag and checked
after every build. It is shareable by link and invisible to search engines
until the dataset and the residency entries are ready to be read by strangers.
[`docs/deploying.md`](docs/deploying.md) covers the deployment model and the
checklist for turning indexing on.

Deployment is a direct upload rather than a Cloudflare-side git build, because
the snapshot is not in git: a build on their servers would succeed and produce
an empty site.

Because the control plane is not public, the site builds against a **JSON
snapshot** exported from the database (`make snapshot`). Every page shows when
that snapshot was taken. The snapshot is gitignored: a fresh clone has no data
and still builds, rendering empty states instead of failing.

The advisor therefore runs **in the browser**, not against the API — which also
means it works offline and leaves no record of what anyone asked. That is a
second implementation of the rules, so it carries the same protection as the
request signing: a golden fixture in `knowledge/fixtures/` is asserted by both
`api/tests/test_advisor_golden.py` and `web/test/advisor.golden.test.js`, and
drift fails one of them.

### The advisor

```bash
curl -s -X POST localhost:8000/v1/advise \
  -H 'content-type: application/json' \
  -d '{"user_countries":["OM"],"data_classes":["personal"]}'
```

Returns candidate regions ranked by *your* measured latency, each with the
obligations attached and a citation into `knowledge/residency/`. The record is
stored and permalinked, because the artifact someone pastes into a design
review has to keep saying what it said when they pasted it, even after the
latency data moves on.

**Every residency entry is currently `draft`** — written from secondary
sources, not checked against the instruments. The advisor says so in its
warnings, the markdown export carries it in the header, and `sovereign
knowledge check` fails any entry claiming a status its metadata cannot support.
The machinery is real; the legal content is a starting point for verification.

### Operator commands

```bash
uv run sovereign migrate                      # apply pending migrations
uv run sovereign issue-token --uses 5         # mint an enrolment token
uv run sovereign rollup --lookback 30         # recompute 5m and 1h buckets
uv run sovereign partitions --drop-older-than 30
uv run sovereign knowledge check              # validate the knowledge base
```

Or use the Makefile: `make up`, `make serve`, `make probe`, `make stats`,
`make backup`, `make test`.

### What survives an outage

Measurements are written to disk before any upload is attempted, and deleted
only once the server acknowledges them. The agent also caches its config, so a
probe that restarts while the control plane is down keeps measuring on its last
known instructions. Ingest deduplicates on (probe, target, timestamp), so
replaying a batch is free.

Verified end to end: killing the API mid-run, restarting the agent cold, and
bringing the server back replayed every buffered sample with zero duplicate
rows.

## Infrastructure

Needs AWS credentials with permission to create S3 buckets and IAM roles. Run
once, by hand, in this order.

```bash
# 1. State bucket. Pick a globally unique name.
terraform -chdir=infra/bootstrap init
terraform -chdir=infra/bootstrap apply -var state_bucket_name=sovereign-tfstate-CHANGEME

# 2. Main stack, using that bucket for state.
cp infra/aws/terraform.tfvars.example infra/aws/terraform.tfvars
$EDITOR infra/aws/terraform.tfvars

terraform -chdir=infra/aws init \
  -backend-config="bucket=sovereign-tfstate-CHANGEME" \
  -backend-config="key=aws/terraform.tfstate" \
  -backend-config="region=me-central-1"

terraform -chdir=infra/aws apply
```

Then set the output role ARNs as GitHub repository variables
(`AWS_PLAN_ROLE_ARN`, `AWS_DEPLOY_ROLE_ARN`, `TF_STATE_BUCKET`) and uncomment
the `plan` job in `.github/workflows/ci.yml`.

**No AWS access keys are created anywhere in this project.** GitHub Actions
exchanges a short-lived OIDC token for temporary credentials; pull requests get
a read-only plan role, and only `main` can apply.

## Cost

Phase 0 runs at roughly **$5/month** — Route 53, a little S3, free-tier RDS when
it arrives. The budget alarm defaults to a $50 ceiling with alerts at 50%, 80%,
100% and on forecast overrun. Raise it deliberately, never by surprise.

## Conventions

- Decisions with a real alternative get an ADR in `docs/adr/`.
- The methodology is published before the numbers.
- Measurement data stays out of git.
