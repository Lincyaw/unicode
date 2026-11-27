import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node } from '../../api/types';
import { ICON_SMALL } from '../../ui/hint';
import { KindIcon } from './icons';
import { useLinks } from './links';
import { StatusDot } from './StatusDot';
import styles from './marks.module.css';

/** How a node chip is drawn (the prototype's node chip): a salient node, a detail node shown only when expanded, or a folded container. */
export type ChipState = 'default' | 'detail' | 'folded';

/**
 * A node as a small chip: status, kind icon and name. On a canvas it previews on a click and enters its page on a
 * double click; in a list it is a link to its page. The selected node is outlined in the brand colour.
 */
export function NodeChip({ node, label, state = 'default', link = false, status, children, className }: {
  node: Node;
  label?: ReactNode;
  state?: ChipState;
  link?: boolean;
  status?: Node['status'];
  children?: ReactNode;
  className?: string;
}) {
  const links = useLinks();
  const content = (
    <>
      {state !== 'detail' && <StatusDot status={status ?? node.status} />}
      <KindIcon node={node} size={ICON_SMALL} />
      <span className={styles.chipLabel}>{label ?? node.title}</span>
      {children}
    </>
  );
  const classes = clsx(styles.chip, className);
  if (link) {
    return <Link className={classes} data-state={state} to={links.entityHref(node.id)}>{content}</Link>;
  }
  return (
    <button
      type="button" className={classes} data-state={state} aria-pressed={links.selected === node.id}
      onClick={() => links.select(node.id)} onDoubleClick={() => links.open(node.id)}
    >
      {content}
    </button>
  );
}
