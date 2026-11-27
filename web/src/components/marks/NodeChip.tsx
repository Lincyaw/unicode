import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node } from '../../api/types';
import { MarkButton } from '../../ui/controls';
import { KindIcon } from './icons';
import { useLinks, useNodeActions } from './links';
import { StatusDot } from './StatusDot';
import styles from './marks.module.css';

/** How a node chip is drawn (the prototype's node chip): a salient node, a detail node shown only when expanded, or a folded container. */
export type ChipState = 'default' | 'detail' | 'folded';

/**
 * How a chip acts: on a drawing it previews its node on a click and enters its page on a double click (`select`); in
 * a list it is a link to its page (`link`); as the node of the page itself it is no control (`current`).
 */
export type ChipMode = 'select' | 'link' | 'current';

/** A node as a small chip: status, kind icon and name, acting by its mode; the selected node carries the still ∞ edge. */
export function NodeChip({ node, label, state = 'default', mode = 'select', status, children, className }: {
  node: Node;
  label?: ReactNode;
  state?: ChipState;
  mode?: ChipMode;
  status?: Node['status'];
  children?: ReactNode;
  className?: string;
}) {
  const links = useLinks();
  const actions = useNodeActions(node.id);
  const content = (
    <>
      {state !== 'detail' && <StatusDot status={status ?? node.status} />}
      <KindIcon node={node} size="small" />
      <span className={styles.chipLabel}>{label ?? node.title}</span>
      {children}
    </>
  );
  const classes = clsx(styles.chip, className);
  // A chip is a control: the kit's ∞ edge sweeps round it under the pointer and holds still while it is selected.
  const control = clsx(classes, 'loop-edge');
  switch (mode) {
    case 'current':
      return <span className={classes} data-state={state} aria-current="page">{content}</span>;
    case 'link':
      return <Link className={control} data-state={state} to={links.entityHref(node.id)}>{content}</Link>;
    case 'select':
      return <MarkButton className={control} data-state={state} {...actions}>{content}</MarkButton>;
  }
}
