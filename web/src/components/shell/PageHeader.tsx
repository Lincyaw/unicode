import type { ReactNode } from 'react';
import { Hint } from '../../ui/hint';
import { Breadcrumbs, type Crumb } from '../../ui/navigation';
import styles from './shell.module.css';

/**
 * The header: the path to the page and its title, then the view tabs, and the tools (the diagnostic count and the
 * compare button). Each node on the path is a link to its page. A path or title too long for the room ends in an
 * ellipsis; the title's hint holds it whole.
 */
export function PageHeader({ crumbs, title, children, tools }: { crumbs: Crumb[]; title: string; children: ReactNode; tools: ReactNode }) {
  return (
    <div className={styles.header}>
      <div className={styles.heading}>
        <Breadcrumbs crumbs={crumbs} />
        <Hint content={title}><h1 className={styles.title}>{title}</h1></Hint>
      </div>
      {children}
      <div className={styles.tools}>{tools}</div>
    </div>
  );
}
