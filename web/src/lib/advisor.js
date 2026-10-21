// Client-side port of api/sovereign_api/advisor.py.
//
// The site is static and the control plane is not public, so the advisor has
// to run in the browser against the snapshot. That means the same logic exists
// twice, in two languages -- the identical hazard as the request signing in
// ADR 0006, and handled the same way: a golden fixture generated from the
// Python implementation is asserted against this one, so drift fails a test
// rather than silently giving two different answers to the same question.
//
// Any change here must be mirrored in advisor.py and the fixture regenerated.

export const DATA_CLASSES = [
  'personal', 'sensitive', 'health', 'financial', 'government', 'operational',
];

// Worst first. A region's verdict is the worst thing any jurisdiction says
// about it.
export const VERDICT_ORDER = { blocked: 0, unknown: 1, conditional: 2, allowed: 3 };

export const DISCLAIMER =
  'Informational only. This is not legal advice, and it is not a substitute ' +
  'for advice from a qualified practitioner in the relevant jurisdiction. ' +
  'Every claim links to the entry it came from; check the primary source ' +
  'before relying on it.';

function ruleApplies(rule, country, dataClasses) {
  const countries = rule.countries ?? [];
  if (!countries.includes('*') && !countries.includes(country)) return false;
  const classes = rule.data_classes ?? [];
  if (classes.includes('*')) return true;
  return dataClasses.some((c) => classes.includes(c));
}

// A localisation rule is not itself a refusal: it constrains where, and the
// per-region check applies it.
const mapVerdict = (v) => (v === 'localised' ? 'conditional' : v ?? 'allowed');

const verdictName = (order) =>
  Object.keys(VERDICT_ORDER).find((k) => VERDICT_ORDER[k] === order) ?? 'allowed';

function dedupe(obligations) {
  const seen = new Set();
  return obligations.filter((o) => (seen.has(o.key) ? false : seen.add(o.key)));
}

function overallStatus(entries) {
  const values = Object.values(entries);
  if (values.some((e) => e.status === 'draft')) return 'draft';
  if (values.some((e) => e.stale)) return 'stale';
  if (values.every((e) => e.status === 'verified')) return 'verified';
  return 'reviewed';
}

export function advise({
  userCountries, dataClasses, entries, rules, candidateRegions, latency,
  window, probeCountries, now,
}) {
  userCountries = userCountries.map((c) => c.toUpperCase());
  const warnings = [];
  const jurisdictions = [];
  const applicable = [];
  let localisation = null;

  for (const country of userCountries) {
    const entry = entries[country];
    if (!entry) {
      warnings.push(
        `No residency entry for ${country}; it was ignored rather than guessed at.`);
      continue;
    }

    const matched = (rules.rules ?? []).filter((r) =>
      ruleApplies(r, country, dataClasses));
    applicable.push(...matched);

    const obligations = [];
    for (const rule of matched) {
      for (const key of rule.obligations ?? []) {
        const spec = rules.obligations?.[key] ?? {};
        obligations.push({
          key, label: spec.label ?? key, detail: spec.detail ?? '',
        });
      }
    }

    for (const rule of matched) {
      if (rule.verdict === 'localised' && rule.requires_region_in) {
        const allowedHere = new Set(rule.requires_region_in);
        localisation = localisation === null
          ? allowedHere
          : new Set([...localisation].filter((c) => allowedHere.has(c)));
      }
    }

    const worst = matched.length
      ? Math.min(...matched.map((r) => VERDICT_ORDER[mapVerdict(r.verdict)] ?? 3))
      : 3;

    jurisdictions.push({
      country,
      country_name: entry.country_name,
      kb_status: entry.status,
      regulator: entry.regulator,
      instrument: entry.instrument,
      verdict: verdictName(worst),
      obligations: dedupe(obligations),
      notes: matched.filter((r) => r.note).map((r) => r.note.trim()),
      open_questions: entry.open_questions ?? [],
      citation: `knowledge/residency/${country}.md`,
      primary_source: entry.primary_source,
    });

    if (entry.status === 'draft') {
      warnings.push(
        `${entry.country_name} is a DRAFT entry written from secondary sources ` +
        'and not checked against the instrument.');
    }
    if (entry.stale) {
      warnings.push(`${entry.country_name} was last verified ` +
        `${entry.days_since_verified} days ago.`);
    }
  }

  if (localisation !== null && localisation.size === 0) {
    warnings.push(
      'Two localisation rules conflict: no region can satisfy both. ' +
      'This workload may not be lawfully deployable as specified.');
  }

  const worstRuleVerdict = applicable.length
    ? Math.min(...applicable.map((r) => VERDICT_ORDER[mapVerdict(r.verdict)] ?? 3))
    : 3;

  const regions = candidateRegions.map((region) => {
    const reasons = [];
    let verdict = verdictName(worstRuleVerdict);

    if (localisation !== null) {
      const allowed = [...localisation].sort().join('/');
      if (localisation.has(region.country)) {
        reasons.push(`Satisfies the localisation requirement (${allowed}).`);
      } else {
        verdict = 'blocked';
        reasons.push(
          `Localisation requires a region in ${allowed}; ` +
          `this region is in ${region.country}.`);
      }
    }

    if (verdict === 'conditional' && reasons.length === 0) {
      reasons.push('Permitted if the listed obligations are met.');
    }
    if (verdict === 'unknown') {
      reasons.push(
        "At least one jurisdiction's position could not be established. " +
        'Treated as unknown rather than assumed permissive.');
    }
    if (verdict === 'allowed' && reasons.length === 0) {
      reasons.push('No obligation found in the rules for this combination.');
    }

    return {
      target_id: region.id,
      provider: region.provider,
      region: region.region,
      country: region.country,
      city: region.city ?? null,
      verdict,
      reasons,
      latency: latency[region.id] ?? null,
    };
  });

  // A target can have samples and no percentile: if every probe against it
  // failed, p95 is null. Those sort last within their verdict, not first.
  regions.sort((a, b) => {
    const va = (VERDICT_ORDER[a.verdict] ?? 9) * -1;
    const vb = (VERDICT_ORDER[b.verdict] ?? 9) * -1;
    if (va !== vb) return va - vb;
    const pa = a.latency?.p95_ms ?? Infinity;
    const pb = b.latency?.p95_ms ?? Infinity;
    if (pa !== pb) return pa - pb;
    return a.target_id < b.target_id ? -1 : a.target_id > b.target_id ? 1 : 0;
  });

  const measured = regions.filter((r) => r.latency).length;
  if (measured === 0) {
    warnings.push(
      'No latency measurements in this window, so the ranking is by verdict only.');
  }

  return {
    generated_at: now ?? new Date().toISOString(),
    status: overallStatus(entries),
    warnings,
    inputs: { user_countries: userCountries, data_classes: dataClasses },
    jurisdictions,
    regions,
    evidence: {
      window,
      probe_countries: probeCountries,
      regions_with_measurements: measured,
      note: 'Percentiles over successful probes only, quarantined probes excluded.',
    },
    rules_version: rules.version,
    disclaimer: DISCLAIMER,
  };
}
