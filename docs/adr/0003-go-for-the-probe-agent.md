# 3. Go for the probe agent, Python for the API

Date: 2026-10-21

## Status

Accepted

## Context

Two components need a language: the probe agent that runs on hardware we do not
own, and the ingest/API service that runs on our own infrastructure. A single
language across both is tidier.

## Decision

Go for the agent. Python with FastAPI for the API.

## Consequences

The agent must run on a Raspberry Pi on a volunteer's home connection, on a
$4 VPS, and inside a container in five clouds. Go gives a single static binary
with no runtime to install, cross-compiles to arm64 from a laptop, and idles in
about 15 MB. Nobody will install a Python environment on their home server to
help with someone's side project; they will run one binary.

The API serves roughly six writes per second. At that rate, request-handling
performance is irrelevant and development speed is not — this is a side project
with finite evening hours, and the author is still building coding fluency.

The cost is two toolchains, two CI jobs, and a serialisation contract that has
to stay in sync. Accepted deliberately. If the split becomes painful, collapse
toward Go rather than Python: the agent's constraints are hard, the API's are not.
