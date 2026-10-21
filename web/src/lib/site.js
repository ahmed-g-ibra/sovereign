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
  repo: 'https://github.com/ahmed-g-ibra/sovereign',
  maintainer: 'Ahmed Gamal',
};

// A published address, set at build time so it is not hard-coded into the
// repository where scrapers find it:
//
//   PUBLIC_CONTACT_EMAIL=you@example.com npm run build
//
// Unset, the contact page routes everything through GitHub issues, which is
// the better channel for corrections anyway: public, threaded and citable.
// The build refuses to produce an indexable site without one, because a
// dataset nobody can challenge is not evidence.
export const contactEmail = import.meta.env.PUBLIC_CONTACT_EMAIL || null;

// Prefix an internal path with the deployment's base.
//
// Astro rewrites asset URLs for you but leaves hand-written hrefs alone, so
// every internal link goes through here. Without it a project-site deploy at
// /sovereign/ yields a site where nothing but the home page resolves.
export function u(path = '/') {
  const base = import.meta.env.BASE_URL || '/';
  if (!path.startsWith('/')) return path;
  return (base.endsWith('/') ? base.slice(0, -1) : base) + path;
}
