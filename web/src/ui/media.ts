import { useSyncExternalStore } from 'react';

export const NARROW_QUERY = '(max-width: 760px)';
export const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

/** The nav's height, from the site's layout tokens. */
export function navHeight(): number {
  return parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--nav-height'));
}

/** How far below the window's top a heading counts as being read: the nav and the anchor gap. */
export function readingLine(): number {
  return navHeight() + parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--anchor-gap'));
}

export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    notify => {
      const list = matchMedia(query);
      list.addEventListener('change', notify);
      return () => list.removeEventListener('change', notify);
    },
    () => matchMedia(query).matches,
  );
}
