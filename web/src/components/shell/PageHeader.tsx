import type { ReactNode } from 'react';
import { Breadcrumbs, type Crumb } from '../../ui/navigation';
import styles from './shell.module.css';

/**
 * The header: the path to the page and its title, the view tabs, the diagnostic count and the compare button. Each
 * node on the path is a link to its page.
 */
export function PageHeader({ crumbs, title, children }: { crumbs: Crumb[]; title: ReactNode; children: ReactNode }) {
  return (
    <div className={styles.header}>
      <div className={styles.heading}>
        <Breadcrumbs crumbs={crumbs} />
        <h1 className={styles.title}>{title}</h1>
      </div>
      {children}
    </div>
  );
}
