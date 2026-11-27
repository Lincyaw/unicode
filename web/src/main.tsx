import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router';
import { router } from './pages/router';
import { HintProvider } from './ui/hint';
import './ui/base.css';

/** The mode `pnpm dev:serve` runs in, which proxies the interface to `unicode serve` instead of the mock. */
const SERVE_MODE = 'serve';

// During development without `unicode serve`, the mock server answers /api in the page's service worker; a build
// leaves it out, since `unicode serve` answers there.
if (import.meta.env.DEV && import.meta.env.MODE !== SERVE_MODE) {
  const { worker } = await import('./mocks/browser');
  await worker.start({ quiet: true });
}

/** Answers stay fresh until the reader moves on: the snapshot changes only when it is rebuilt. */
const queryClient = new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, retry: false } } });

const root = document.getElementById('root');
if (!root) {
  throw new Error('index.html has the #root element the viewer renders into');
}
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <HintProvider>
        <RouterProvider router={router} />
      </HintProvider>
    </QueryClientProvider>
  </StrictMode>,
);
