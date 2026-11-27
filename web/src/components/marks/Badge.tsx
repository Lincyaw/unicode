import { clsx } from 'clsx';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { ICON_STROKE, ICON_TINY } from '../../ui/hint';
import styles from './marks.module.css';

/** A badge's colour by what it says: neutral data, a warning, a success, or information such as a link. */
export type Tone = 'neutral' | 'warning' | 'success' | 'info' | 'caution';

/**
 * A small tag (the prototype's tag): a label, a count or a link, with an optional icon. A badge with `to` is a link and takes
 * the info tone's hover.
 */
export function Badge({ tone, icon: Icon, to, mono = false, children, className }: {
  tone: Tone; icon?: LucideIcon; to?: string; mono?: boolean; children?: ReactNode; className?: string;
}) {
  const content = (
    <>
      {Icon && <Icon size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />}
      {children}
    </>
  );
  const classes = clsx(styles.badge, mono && styles.mono, className);
  return to === undefined
    ? <span className={classes} data-tone={tone}>{content}</span>
    : <Link className={classes} data-tone={tone} to={to}>{content}</Link>;
}
