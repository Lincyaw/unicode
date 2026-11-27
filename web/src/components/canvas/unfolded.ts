import { useState } from 'react';

/** The type cards the reader has unfolded to all their fields on this drawing; an unfolded card stays unfolded. */
export function useUnfolded(): { unfolded: ReadonlySet<string>; unfold: (id: string) => void } {
  const [unfolded, setUnfolded] = useState<ReadonlySet<string>>(new Set());
  return { unfolded, unfold: id => setUnfolded(current => new Set(current).add(id)) };
}
