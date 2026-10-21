# Probe agent

Zero external dependencies — standard library only. That is a deliberate
constraint: this binary has to be trivially auditable by anyone asked to run it
on their own machine, and `go.sum` conflicts are not a conversation worth having
with a volunteer.

```
cmd/probe          CLI entry point
internal/probe     measurement + percentile aggregation
internal/targets   target list loading and filtering
targets.json       the endpoints, as data
```

Phase 1 adds: registration against a control plane, signed batch upload,
local buffering across outages, host-health reporting, and traceroute.
