import { useState } from 'react';

/** The first `limit` items of a list, and whether the reader has unfolded the rest with its +n. */
export function useFolded<T>(items: T[], limit: number | undefined): { shown: T[]; more: number; unfold: () => void } {
  const [open, setOpen] = useState(false);
  const shown = open || limit === undefined ? items : items.slice(0, limit);
  return { shown, more: items.length - shown.length, unfold: () => setOpen(true) };
}
