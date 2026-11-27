import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import styles from './marks.module.css';

/** Lifts the controls it holds, such as a hint or a toggle, above the cover of their box (CoverButton). */
export function Above({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={clsx(styles.above, className)} data-lifted>{children}</span>;
}
