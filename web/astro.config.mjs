import { defineConfig } from 'astro/config';

// Fully static. There is no server to run, which is the point: the site costs
// nothing to host and has nothing to keep alive. Data arrives as a JSON
// snapshot exported from the control plane at build time.
// GitHub Pages serves a project site from /<repo>/, Cloudflare Pages serves
// from the root, and a custom domain serves from the root again. The base is
// therefore a deploy-time decision, not a property of the site.
//
//   npm run build                      -> served at /
//   PUBLIC_BASE=/sovereign npm run build -> served at /sovereign/
export default defineConfig({
  output: 'static',
  site: process.env.PUBLIC_SITE_URL || 'https://sovereign.example',
  base: process.env.PUBLIC_BASE || '/',
  trailingSlash: 'always',
  build: { format: 'directory' },
  devToolbar: { enabled: false },
});
