// Site-wide build flags.
//
// Indexing is off by default and must be turned on deliberately. The site
// publishes draft legal content and a dataset from a single probe; neither
// should be discoverable until it can stand up to a stranger reading it.
//
//   npm run build                        -> noindex (shareable by link only)
//   PUBLIC_INDEXABLE=true npm run build  -> indexable
//
// The same flag drives the meta tag, robots.txt and the X-Robots-Tag header,
// all generated from here, because three hand-maintained copies of one
// decision is three chances to disagree.
export const indexable = import.meta.env.PUBLIC_INDEXABLE === 'true';

export const site = {
  name: 'Sovereign',
  tagline: 'Independent latency and data-residency measurement for Gulf cloud regions',
};
