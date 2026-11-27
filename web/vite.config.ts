import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/*
 * `pnpm dev` answers the interface from the mock server in the page's service worker. `pnpm dev:serve` (mode serve)
 * starts no mock and proxies /api to `unicode serve`, at UNICODE_SERVE or its default address.
 */
export const SERVE_MODE = 'serve';
export const DEFAULT_SERVE_ADDRESS = 'http://127.0.0.1:7090';

export default defineConfig(({ command, mode }) => ({
  plugins: [react()],
  // public/ holds only the mock's service worker, which a build for unicode serve leaves out.
  publicDir: command === 'serve' ? 'public' : false,
  // unicode serve hands the bundle over localhost, so one chunk of this size loads at once; the default limit is for the web.
  build: { chunkSizeWarningLimit: 800 },
  server: mode === SERVE_MODE ? { proxy: { '/api': process.env.UNICODE_SERVE ?? DEFAULT_SERVE_ADDRESS } } : {},
}));
