# Running Sovereign at zero cost

Everything works on one machine with no cloud account, no domain and no spend.
What you give up is reach — one vantage point instead of a fleet, and an API
nobody else can call. What you keep is the only thing that cannot be bought
later: **history**.

A probe running at home in Muscat today produces exactly the same rows the
deployed version would. In six months that is a dataset. Starting in six months
is starting from zero.

## Once

```bash
make up                 # Postgres on 55432, migrations applied
make serve              # control plane on :8000, leave this running
make token              # copy the token it prints

cd agent
go build -o bin/probe ./cmd/probe
./bin/probe -register -server http://127.0.0.1:8000 \
  -token "PASTE_TOKEN" -name muscat-home -country OM -city Muscat \
  -isp Omantel -tier core
```

## Then, so it survives reboots

```bash
ops/install-macos.sh
```

That installs three launchd jobs: the stack (Postgres + API), the probe, and a
rollup that runs every five minutes. Logs land in `data/logs/`. Remove them
with `ops/install-macos.sh --remove`.

On a Linux probe host, use `ops/systemd/sovereign-probe.service` instead.

## Day to day

```bash
make stats        # how much history exists
make backup       # pg_dump to data/backups, keeps the last 14
make verify-targets
make test
```

## What to expect

A single probe against 11 live targets produces about 16,000 samples a day,
roughly 6 MB a month in Postgres before rollups. A laptop that sleeps overnight
produces a gap, which is honest: the methodology treats a silent probe as
*unknown*, never as *down*.

## The limits you are accepting

| Limit | Consequence |
|---|---|
| One vantage point | No cross-ISP or cross-country comparison yet |
| Measurements only while the machine is awake | Gaps, correctly recorded as unknown |
| Public regional endpoints, not our own responders | Vendor controls what sits behind the name (ADR 0005) |
| No Azure per-region numbers | Azure appears only as a global control |
| Local backups only | A dead disk loses the history; copy `data/backups/` somewhere else |

That last one is worth taking seriously. The entire value of running free is
the history you are accumulating, and it currently lives on exactly one disk.

## What this is not

This is not a staging environment for a deployment that already exists. It is
the deployment, scoped to what a free setup can honestly do. When budget
arrives, `docs/costs.md` lists what to buy and in what order; the application
code does not change.
