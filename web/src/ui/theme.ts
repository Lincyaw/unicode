import { useSyncExternalStore } from 'react';

/** Where the reader's theme choice is kept; index.html reads the same key before the first paint. */
export const THEME_STORAGE_KEY = import.meta.env.VITE_THEME_STORAGE_KEY;

export type Theme = 'light' | 'dark';

/** The theme the page is in; index.html sets it before the first paint, so the page is always in one. */
function currentTheme(): Theme {
  const theme = document.documentElement.dataset.theme;
  if (theme !== 'light' && theme !== 'dark') {
    throw new Error(`The page is in no theme: <html data-theme> is ${String(theme)}, and index.html sets it before the first paint`);
  }
  return theme;
}

export function toggleTheme() {
  const next = currentTheme() === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  localStorage.setItem(THEME_STORAGE_KEY, next);
}

/** The theme the page is in, following every switch, and the switch itself; a page hands both to its theme control. */
export function useTheme(): [Theme, () => void] {
  const theme = useSyncExternalStore(
    notify => {
      const observer = new MutationObserver(notify);
      observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
      return () => observer.disconnect();
    },
    currentTheme,
  );
  return [theme, toggleTheme];
}
