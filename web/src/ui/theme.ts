/** Where the reader's theme choice is kept; index.html reads the same key before the first paint. */
export const THEME_STORAGE_KEY = import.meta.env.VITE_THEME_STORAGE_KEY;

export function toggleTheme() {
  const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  localStorage.setItem(THEME_STORAGE_KEY, next);
}
