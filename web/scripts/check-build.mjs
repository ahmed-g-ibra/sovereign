// Verify the built site agrees with the indexing flag.
//
// The flag has to be expressed in three places that a deploy depends on: the
// meta tag in every page, the X-Robots-Tag header, and robots.txt. This check
// runs after every build so they cannot silently disagree -- which is the kind
// of mistake you discover from a search result months later.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const dist = resolve(here, '../dist');
const indexable = process.env.PUBLIC_INDEXABLE === 'true';

function htmlFiles(dir) {
  return readdirSync(dir).flatMap((name) => {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) return htmlFiles(full);
    return name.endsWith('.html') ? [full] : [];
  });
}

const problems = [];
const pages = htmlFiles(dist);

if (pages.length === 0) problems.push('no HTML pages in dist/');

for (const page of pages) {
  const html = readFileSync(page, 'utf8');
  const hasNoindex = /name="robots"[^>]*noindex/.test(html);
  const rel = page.replace(dist + '/', '');
  if (indexable && hasNoindex) problems.push(`${rel}: noindex present but indexing is on`);
  if (!indexable && !hasNoindex) problems.push(`${rel}: missing the noindex meta tag`);
}

const headers = readFileSync(join(dist, '_headers'), 'utf8');
const headerNoindex = headers.includes('X-Robots-Tag: noindex');
if (indexable && headerNoindex) problems.push('_headers: X-Robots-Tag set but indexing is on');
if (!indexable && !headerNoindex) problems.push('_headers: missing X-Robots-Tag: noindex');

const robots = readFileSync(join(dist, 'robots.txt'), 'utf8');
if (/Disallow:\s*\/\s*$/m.test(robots)) {
  problems.push('robots.txt disallows crawling, which would hide the noindex directive');
}

if (problems.length) {
  console.error(`build check FAILED (indexable=${indexable}):`);
  for (const p of problems) console.error(`  - ${p}`);
  process.exit(1);
}
console.log(`build check passed: ${pages.length} pages, indexable=${indexable}`);
