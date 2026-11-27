import { setupWorker } from 'msw/browser';
import { handlers } from './handlers';

/** The mock of `unicode serve`, answering the page's /api requests from its service worker during development. */
export const worker = setupWorker(...handlers);
