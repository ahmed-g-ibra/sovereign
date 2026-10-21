// Copy repository documentation into the site as Astro markdown pages.
//
// The methodology is normative: it is the contract the measurements are held
// to. Keeping one copy in docs/ and generating the page from it means the
// published page cannot drift from the document contributors review.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../..');

const pages = [
  {
    src: `${repo}/docs/methodology.md`,
    dest: `${here}/../src/pages/methodology.md`,
    title: 'Methodology',
    description: 'What Sovereign measures, how, and the rules it holds itself to.',
    current: '/methodology/',
  },
];

for (const page of pages) {
  const body = readFileSync(page.src, 'utf8').replace(/^#\s+.*\n/, '');
  const frontmatter = [
    '---',
    'layout: ../layouts/Doc.astro',
    `title: ${JSON.stringify(page.title)}`,
    `description: ${JSON.stringify(page.description)}`,
    `current: ${JSON.stringify(page.current)}`,
    '---',
    '',
    '<!-- Generated from docs/ by scripts/sync-docs.mjs. Do not edit. -->',
    '',
  ].join('\n');

  mkdirSync(dirname(page.dest), { recursive: true });
  writeFileSync(page.dest, frontmatter + body);
  console.log(`synced ${page.src.replace(repo + '/', '')} -> ${page.dest.split('/web/')[1]}`);
}
