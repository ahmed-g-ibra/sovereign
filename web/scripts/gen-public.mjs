// Generate robots.txt and the Cloudflare _headers file from one flag.
//
// A subtlety worth stating, because it is the usual mistake: to keep a page
// out of a search index you must ALLOW crawling and serve noindex. Blocking
// the crawler in robots.txt means it never reads the noindex, and the URL can
// still be listed from inbound links alone. So robots.txt stays permissive and
// the work is done by the meta tag and the X-Robots-Tag header.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const publicDir = resolve(here, '../public');
const indexable = process.env.PUBLIC_INDEXABLE === 'true';

mkdirSync(publicDir, { recursive: true });

const robots = indexable
  ? `# Indexing enabled.
User-agent: *
Allow: /
`
  : `# Crawling is allowed on purpose: a crawler blocked here would never read
# the noindex directive and could still list the URL from inbound links.
# Indexing is refused by the robots meta tag and the X-Robots-Tag header.
User-agent: *
Allow: /
`;

const headers = `/*
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin
  X-Frame-Options: DENY${indexable ? '' : '\n  X-Robots-Tag: noindex, nofollow'}

/_astro/*
  Cache-Control: public, max-age=31536000, immutable
`;

writeFileSync(resolve(publicDir, 'robots.txt'), robots);
writeFileSync(resolve(publicDir, '_headers'), headers);
console.log(`generated robots.txt and _headers (indexable=${indexable})`);
