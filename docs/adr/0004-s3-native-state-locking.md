# 4. S3 native state locking, no DynamoDB table

Date: 2026-10-21

## Status

Accepted

## Context

Every Terraform guide written before late 2024 pairs the S3 backend with a
DynamoDB table for state locking. Terraform 1.10 added native S3 locking via
`use_lockfile`, which writes a lock object beside the state file.

## Decision

Use `use_lockfile = true`. No DynamoDB table.

## Consequences

- One less resource to create, tag, pay for and explain.
- Requires Terraform 1.10 or newer, pinned in `required_version`.
- Anyone reviewing this repo against an older tutorial will ask where the lock
  table is; this file is the answer.
