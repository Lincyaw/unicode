import type { ReactNode } from 'react';
import { Breadcrumbs, type Crumb } from '../../ui/navigation';
import styles from './shell.module.css';

/**
 * The header: the path to the page and its title, then the view tabs, and the tools (the diagnostic count and the
 * compare button). Each node on the path is a link to its page. A path or title too long for the room wraps, so it is
 * read whole by every reader; on a narrow window the tabs scroll in their own track.
 */
export function PageHeader({ crumbs, title, children, tools }: { crumbs: Crumb[]; title: ReactNode; children: ReactNode; tools: ReactNode }) {
  return (
    <div className={styles.header}>
      <div className={styles.heading}>
        <Breadcrumbs crumbs={crumbs} />
        <h1 className={styles.title}>{title}</h1>
      </div>
      <div className={styles.tabs}>{children}</div>
      <div className={styles.tools}>{tools}</div>
    </div>
  );
}
