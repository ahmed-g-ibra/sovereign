export const ms = (v) => (v === null || v === undefined ? '—' : `${v.toFixed(1)}`);

export const pct = (v) => (v === null || v === undefined ? '—' : `${v.toFixed(1)}%`);

export function ago(iso) {
  if (!iso) return 'never';
  const seconds = (Date.now() - new Date(iso).getTime()) / 1000;
  if (seconds < 90) return 'just now';
  const units = [
    [60, 'minute'],
    [24, 'hour'],
    [30, 'day'],
    [12, 'month'],
  ];
  let value = seconds / 60;
  let unit = 'minute';
  for (const [size, next] of units) {
    if (value < size) break;
    value /= size;
    unit = next;
  }
  const n = Math.floor(value);
  return `${n} ${unit}${n === 1 ? '' : 's'} ago`;
}

export const flag = (cc) =>
  !cc || cc.length !== 2 || cc === 'XX'
    ? '🌐'
    : String.fromCodePoint(...[...cc.toUpperCase()].map((c) => 0x1f1a5 + c.charCodeAt(0)));

/** Latency bands, used to colour a cell without inventing precision. */
export function band(p50) {
  if (p50 === null || p50 === undefined) return 'none';
  if (p50 < 60) return 'fast';
  if (p50 < 150) return 'ok';
  if (p50 < 400) return 'slow';
  return 'bad';
}

export const providerName = (p) =>
  ({ aws: 'AWS', gcp: 'Google', azure: 'Azure', oci: 'Oracle' })[p] ?? p;
