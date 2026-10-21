// The JavaScript advisor must produce exactly what the Python one does.
//
// Same fixture, asserted on both sides. If someone edits one implementation
// and not the other, one of these two tests fails -- which is the entire
// reason the fixture exists.
//
//     npm test
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

import { advise } from '../src/lib/advisor.js';

const here = dirname(fileURLToPath(import.meta.url));
const fixture = JSON.parse(
  readFileSync(resolve(here, '../../knowledge/fixtures/advisor-golden.json'), 'utf8')
);

for (const testCase of fixture.cases) {
  test(`advisor agrees with Python: ${testCase.name}`, () => {
    const got = advise({
      userCountries: testCase.user_countries,
      dataClasses: testCase.data_classes,
      entries: fixture.entries,
      rules: fixture.rules,
      candidateRegions: fixture.candidate_regions,
      latency: fixture.latency,
      window: fixture.window,
      probeCountries: fixture.probe_countries,
      now: 'fixed',
    });
    delete got.generated_at;

    assert.deepEqual(
      JSON.parse(JSON.stringify(got)),
      testCase.expected,
      `the JavaScript advisor disagrees with the Python one for ` +
      `"${testCase.name}". Reconcile web/src/lib/advisor.js with ` +
      `api/sovereign_api/advisor.py.`
    );
  });
}

test('fixture covers the cases that have broken before', () => {
  const names = fixture.cases.map((c) => c.name).join(' | ');
  assert.match(names, /localisation/, 'no localisation case');
  assert.match(names, /unknown/, 'no unknown-jurisdiction case');
  assert.match(names, /no entry/, 'no missing-country case');

  // A target with samples but no percentile: every probe against it failed.
  const lossy = Object.values(fixture.latency).find((l) => l.p95_ms === null);
  assert.ok(lossy, 'fixture should include a fully lossy target');
  assert.equal(lossy.loss_pct, 100);
});
