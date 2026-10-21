# 5. Never measure an endpoint we do not control or that a vendor does not publish

Date: 2026-10-21

## Status

Accepted

## Context

Azure publishes no consistent public per-region endpoint, so Phase 0 tested a
hypothesis: that `<region>.blob.core.windows.net` would resolve, on the theory
that someone has created a storage account named after each region.

The first verify pass found `uaenorth.blob.core.windows.net` returns NXDOMAIN,
while `qatarcentral.blob.core.windows.net` **resolves and completes a TLS
handshake in ~635 ms**.

The tempting conclusion is that we have a Qatar endpoint.

## Decision

Reject it. That name is a storage account some unrelated person or company
happens to have called `qatarcentral`. It is not an Azure regional endpoint.

Both entries stay in `targets.json` with status `rejected` and the reason, so
the idea is not rediscovered and re-litigated in three months.

## Consequences

- Azure appears only as a global anycast control until Phase 2 deploys our own
  in-region responders. The site must say so plainly rather than leave a
  suspicious gap.
- A measurement target must be either an endpoint the provider documents as
  regional, or infrastructure we deployed ourselves. Nothing else.
- This is the argument for Phase 2 responders in one concrete example: the
  alternative is building a public dataset on a stranger's storage account that
  can disappear, move region, or get a CDN in front of it without warning.
- Reachability is not validity. The verify pass answers "does this respond",
  never "does this mean what I think it means".
