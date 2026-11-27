import { clsx } from 'clsx';
import type { LucideIcon } from './icons';
import type { ReactNode } from 'react';
import { Link } from 'react-router';

import styles from './marks.module.css';
import { Icon } from '../../ui/icons';

/** A badge's colour by what it says: neutral data, a warning, a success, or information such as a link. */
export type Tone = 'neutral' | 'warning' | 'success' | 'info' | 'caution';

/**
 * A small tag (the prototype's tag): a label, a count or a link, with an optional icon. A badge with `to` is a link, one
 * with `onClick` a button, such as a +n that unfolds a list; both take the hover of a control.
 */
export function Badge({ tone, icon: glyph, to, onClick, mono = false, children, className }: {
  tone: Tone; icon?: LucideIcon; to?: string; onClick?: () => void; mono?: boolean; children?: ReactNode; className?: string;
}) {
  const content = (
    <>
      {glyph && <Icon glyph={glyph} size="tiny" />}
      {children}
    </>
  );
  const classes = clsx(styles.badge, mono && styles.mono, className);
  if (to !== undefined) {
    return <Link className={classes} data-tone={tone} to={to}>{content}</Link>;
  }
  if (onClick !== undefined) {
    return <button type="button" className={classes} data-tone={tone} onClick={onClick}>{content}</button>;
  }
  return <span className={classes} data-tone={tone}>{content}</span>;
}
