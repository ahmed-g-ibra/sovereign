# Deploying the site

Cloudflare Pages, free tier, direct upload. No build minutes, no bandwidth
bill, no server. See [`costs.md`](costs.md) for what is still deferred.

## Why direct upload rather than a git build

Cloudflare can build from a connected repository on every push, and that is
the usual setup. It is wrong here.

The site renders a **snapshot exported from the control plane**, and that
snapshot is deliberately not in git. A build running on Cloudflare's servers
would therefore succeed and produce a site with no data in it — correct,
empty, and misleading.

So deployment is a push from a machine that has the database: your laptop,
today. The exported data goes up with the HTML, and the site shows the date it
was taken.

If that becomes annoying, the alternative is committing snapshots on a
schedule, which trades repository churn for hands-off deploys. Not worth it
for one probe.

## One-time setup

1. Create a Cloudflare account (free) and a Pages project named `sovereign`.
   Choose **direct upload**, not a git connection.
2. Authenticate once:

   ```bash
   cd web && npx wrangler login
   ```

## Deploying

```bash
make deploy
```

That exports a fresh snapshot, builds, verifies the output, and uploads. You
get a `*.pages.dev` URL immediately.

To deploy without refreshing the data — publishing a copy change, say:

```bash
cd web && npm run deploy
```

## Indexing

The site ships **unlisted**: shareable by link, refused by search engines. It
publishes draft legal content and a dataset from a single vantage point, and
neither should be discoverable until it can stand up to a stranger reading it.

One flag drives all three places the decision has to be expressed:

| Where | Unlisted | Indexed |
|---|---|---|
| `<meta name="robots">` on every page | `noindex, nofollow` | absent |
| `X-Robots-Tag` response header | `noindex, nofollow` | absent |
| `robots.txt` | `Allow: /` | `Allow: /` |

`robots.txt` stays permissive in **both** modes, and that is deliberate. To
keep a page out of an index you must let the crawler fetch it and serve
`noindex`; a crawler blocked by `robots.txt` never reads the directive, and the
URL can still be listed from inbound links alone. Blocking in `robots.txt` is
the common mistake and it achieves the opposite of what people expect.

`scripts/check-build.mjs` runs after every build and fails if the three
disagree, so this cannot drift quietly.

### Turning indexing on

When the dataset has months behind it and the residency entries are no longer
drafts:

```bash
cd web && PUBLIC_INDEXABLE=true npm run deploy
```

Before you do, work through:

- [ ] At least two vantage points, so no figure rests on one probe
- [ ] Enough history that the percentiles mean something — weeks, not days
- [ ] Residency entries moved off `draft`, or the drafts clearly quarantined
- [ ] A contact address that reaches you
- [ ] The methodology page read end to end as a stranger would read it

Indexing is easy to turn on and slow to undo: search engines cache, and
screenshots travel. There is no hurry.

## Custom domain

Add it in the Pages project settings. A domain is the only recurring cost in
this entire setup, around $10–15 a year.

## What this costs

Nothing. Cloudflare Pages' free tier covers unlimited requests and bandwidth,
500 builds a month — and direct upload does not consume build minutes at all.
