import { useState } from 'react';

/**
 * A size the reader chose, such as a column's width, kept in the browser under a key and held within bounds; a size
 * kept from a wider window is brought back within them. It is the reader's preference, so the address does not carry it.
 */
export function useStoredSize(key: string, initial: number, min: number, max: number): [number, (size: number) => void] {
  const clamp = (size: number) => Math.min(max, Math.max(min, size));
  const [size, setSize] = useState(() => {
    const kept = Number(localStorage.getItem(key));
    return Number.isFinite(kept) && kept > 0 ? kept : initial;
  });
  return [clamp(size), next => {
    const bounded = clamp(next);
    localStorage.setItem(key, String(bounded));
    setSize(bounded);
  }];
}
