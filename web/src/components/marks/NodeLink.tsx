import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node } from '../../api/types';
import { useLinks } from './links';
import styles from './marks.module.css';

/** How a link to a node reads: in the text colour, as a link, muted like a path, or as the text around it. */
export type LinkTone = 'main' | 'link' | 'muted' | 'inherit';

/** A link to a node's page, set in the code face. */
export function NodeLink({ node, label, tone = 'link', className }: { node: Pick<Node, 'id' | 'title'>; label?: ReactNode; tone?: LinkTone; className?: string }) {
  const links = useLinks();
  return <Link className={clsx(styles.nodeLink, className)} data-tone={tone} to={links.entityHref(node.id)}>{label ?? node.title}</Link>;
}
