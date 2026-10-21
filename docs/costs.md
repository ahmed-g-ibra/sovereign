# Deferred spend register

Nothing in this project currently costs anything. Every item below is **not
provisioned** and is deliberately deferred until there is budget.

Prices are AWS list prices for **me-central-1 (UAE)** pulled from the AWS Price
List API on 2026-10-21, at 730 hours/month. The UAE region runs roughly 30%
above us-east-1, so figures quoted in most tutorials are too low.

## 1. AWS — the deployment

| Item | Rate | Monthly | Status |
|---|---|---|---|
| ALB | $0.028224/hr | $20.60 | **deferred** |
| Fargate ARM vCPU (0.25) | $0.04208/vCPU-hr | $7.68 | **deferred** |
| Fargate ARM memory (0.5 GB) | $0.00464/GB-hr | $1.69 | **deferred** |
| RDS `db.t4g.micro` PostgreSQL, single-AZ | $0.019/hr | $13.87 | **deferred** |
| RDS gp3 storage, 20 GB | ~$0.138/GB-mo *(derived)* | ~$2.80 | **deferred** |
| NAT Gateway | ~$0.059/hr *(derived)* | ~$43.00 | **avoid entirely** |
| Public IPv4 address | $0.005/hr | $3.65 each | **deferred** |
| S3 standard | $0.025/GB-mo | <$1 | **deferred** |
| Route 53 hosted zone | $0.50/zone | $0.50 | **deferred** |
| Secrets Manager | $0.40/secret | $0.80 | **deferred** |
| CloudWatch logs | ingest + storage | ~$1–2 | **deferred** |

Derived figures are extrapolated from the regional premium and carry ±15%.
Verify in the AWS Pricing Calculator before committing.

**Deployment shapes priced:**

| | Architecture | Monthly |
|---|---|---|
| A | ALB → Fargate → RDS | ~$55 |
| B | API Gateway → Lambda → RDS | ~$23 |
| D | Cloudflare → Fargate with public IP → RDS *(recommended)* | ~$33 |

The NAT Gateway is the single largest avoidable line in the project — larger
than the database and compute combined. A public subnet with a security group
replaces it at no cost. Most "production-grade AWS" guides include one.

## 2. Probe fleet

| Item | Cost | Status |
|---|---|---|
| GCC VPS vantage points (4–6 needed) | $5–15/mo each → **$30–50/mo** | **deferred** |
| Phase 2 in-region responders (8–10 across 4 clouds) | $4–5/mo each → **$40–50/mo** | **deferred** |

This will eventually exceed the AWS bill. It is also the line that community
probes are designed to replace: a contributor running the agent on their own
connection costs nothing and adds ISP diversity we could not buy anyway.

## 3. Other

| Item | Cost | Status |
|---|---|---|
| Domain registration | $10–15/year | **deferred** |
| Cloudflare (free tier: TLS, CDN, DDoS) | $0 | available now |
| GitHub Actions on a public repo | $0 | available now |

## 4. What the deferral actually blocks

| Blocked | Free substitute in place |
|---|---|
| 24/7 ingest reachable from anywhere | local stack via `docker compose`, run on your own machine |
| Probes in UAE / Saudi / Qatar / Bahrain | one probe at home in Oman |
| Measuring from inside cloud regions | public regional endpoints (Phase 1 approach) |
| A public website on a real domain | the API and site run on localhost |
| Managed backups, Multi-AZ, alerting | `pg_dump` to a local disk |

**What the deferral does not block:** accumulating measurement history. A probe
running at home against a local Postgres produces exactly the same data the
deployed version would. History is the one asset that cannot be backfilled, and
it costs nothing to start today. See `docs/running-free.md`.

## 5. When budget arrives

Suggested order, highest value first:

1. **Domain + Cloudflare** ($15/yr) — makes everything else presentable.
2. **Option D deployment** (~$33/mo) — the probe reports 24/7 instead of only
   while your laptop is on.
3. **Two GCC vantage points** (~$20/mo) — one in UAE, one in Saudi. This is the
   point where the dataset becomes defensible rather than anecdotal.
4. **In-region responders** (~$45/mo) — Phase 2, and the answer to ADR 0005.

Set the budget alarm just above the expected run rate at each step. The alarm
currently defaults to $50, which Option A would trip in its first month.

## 6. Free-tier note

A new AWS account gets $100 in credits at sign-up and up to $100 more for
completing onboarding activities. The free plan ends after six months or when
the credits run out, whichever comes first. That covers roughly three to five
months of Option D. Accounts created before 15 July 2025 are on the older
model instead.
