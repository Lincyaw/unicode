import { useState } from 'react';

/** The type cards the reader has unfolded to all their fields on this drawing. */
export function useUnfolded(): { full: ReadonlySet<string>; toggle: (id: string) => void } {
  const [full, setFull] = useState<ReadonlySet<string>>(new Set());
  const toggle = (id: string) => setFull(current => {
    const next = new Set(current);
    if (!next.delete(id)) {
      next.add(id);
    }
    return next;
  });
  return { full, toggle };
}
