// Snapshot loader.
//
// import.meta.glob returns an empty object when no files match, so a checkout
// with no snapshot still builds and every page renders an empty state. That
// matters: a contributor cloning this repo has no database, and a site that
// refuses to build without one is a site nobody will contribute to.
const files = import.meta.glob('../data/*.json', { eager: true, import: 'default' });

const pick = (name, fallback) => files[`../data/${name}.json`] ?? fallback;

export const meta = pick('meta', {
  generated_at: null,
  window: '7 days',
  kb_status: 'draft',
  // Mirrors api/sovereign_api/licensing.py. The snapshot carries its own
  // licence; this is only the fallback for a build with no data.
  license: {
    name: 'CC BY 4.0',
    url: 'https://creativecommons.org/licenses/by/4.0/',
    attribution: 'Sovereign — https://github.com/ahmed-g-ibra/sovereign',
  },
  stats: { samples: 0, traces: 0, probes: 0, oldest: null, newest: null },
});

export const matrix = pick('matrix', []);
export const paths = pick('paths', []);
export const changes = pick('changes', []);
export const networks = pick('networks', []);
export const targets = pick('targets', []);
export const probes = pick('probes', []);
export const residency = pick('residency', {});
export const rules = pick('rules', { version: 0, obligations: {}, rules: [] });

export const hasData = matrix.length > 0 || paths.length > 0;

/** Regions you could actually deploy to, as opposed to measurement controls. */
export const deployable = matrix.filter((r) => !r.is_control);
export const controls = matrix.filter((r) => r.is_control);
