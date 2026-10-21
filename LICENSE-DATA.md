# Licence for the measurement data

The **measurement data** published by this project is licensed under
[Creative Commons Attribution 4.0 International (CC BY 4.0)][cc].

You may share and adapt it, including commercially, provided you give credit.

## What this covers

Everything the project measures, and everything derived from it:

- latency samples (DNS, TCP and TLS timings, loss)
- traceroutes, hop addresses and AS paths
- the 5-minute and hourly rollups
- the JSON snapshots the website is built from, and the responses of the
  public API endpoints

Wherever it appears — on the site, in an API response, in a downloaded
snapshot — it is CC BY 4.0.

## What this does not cover

**The code is MIT**, not CC BY. See [LICENSE](LICENSE). That covers the agent,
the API, the website, the Terraform, and the residency entries in
`knowledge/`, which are written prose rather than measurements.

If you are reusing the software, MIT applies. If you are reusing the numbers,
this file applies.

## How to attribute

A link is enough:

> Latency data from [Sovereign](https://github.com/ahmed-g-ibra/sovereign),
> CC BY 4.0.

For a chart, a report or anything with a date attached, say when the data was
taken — every published figure carries a generation timestamp, and latency
measurements age:

> Sovereign, *Gulf cloud latency measurements*, retrieved 2026-10-22.
> CC BY 4.0. https://github.com/ahmed-g-ibra/sovereign

## Why attribution rather than public domain

The value of this dataset is that it can be traced back to a published
methodology and a known set of vantage points. A figure reproduced without
that link is an unsourced number, which is the thing this project exists to
replace. Attribution keeps the measurements auditable by whoever reads them
next.

## What is not promised

No warranty, and no fitness for any particular purpose. These are
measurements from a small number of volunteer vantage points, with the
limitations set out in the [methodology](docs/methodology.md). Several
residency entries are explicitly marked draft. Do not treat any of it as
advice.

[cc]: https://creativecommons.org/licenses/by/4.0/
