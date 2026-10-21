# Sovereign

Independent latency and data-residency intelligence for Gulf cloud regions.

> Where is this workload allowed to live, and what does that choice cost me in
> latency?

This repository is **Phase 0**: a Terraform skeleton, CI/CD with no long-lived
AWS credentials, a cost alarm, and a probe that measures real latency from a
Gulf connection to every Middle East cloud region.

Full plan: [`docs/build-plan.md`](docs/build-plan.md)

## Why the clock matters

AWS is targeting **December 2026** for its first Saudi region and Azure's
**Saudi Arabia East** is slated for Q4 2026. Probes running before launch day
produce the first independent benchmarks for both — numbers that cannot be
generated retroactively. Everything in Phase 0 exists to make those launches a
one-line change to `agent/targets.json`.

## Layout

```
agent/      Go probe: times DNS, TCP and TLS to cloud regional endpoints
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
