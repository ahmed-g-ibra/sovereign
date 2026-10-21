import { defineConfig } from 'astro/config';

// Fully static. There is no server to run, which is the point: the site costs
// nothing to host and has nothing to keep alive. Data arrives as a JSON
// snapshot exported from the control plane at build time.
export default defineConfig({
  output: 'static',
  site: 'https://sovereign.example',
  build: { format: 'directory' },
  devToolbar: { enabled: false },
});
