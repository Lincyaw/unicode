import { clsx } from 'clsx';
import type { Status } from '../../api/types';
import { STATUS_NAMES } from '../../terms';
import styles from './marks.module.css';

/**
 * The status of a declared item as a dot: filled green verified, filled yellow missing, hollow stub, filled red
 * contradicted. Its name is there for screen readers; an item without a status keeps the dot's place empty.
 */
export function StatusDot({ status, className }: { status?: Status; className?: string }) {
  if (status === undefined) {
    return <span className={clsx(styles.dot, className)} aria-hidden="true" />;
  }
  return <span className={clsx(styles.dot, className)} data-status={status} role="img" aria-label={STATUS_NAMES[status]} />;
}
