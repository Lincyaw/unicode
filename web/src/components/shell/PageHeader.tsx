import type { ReactNode } from 'react';
import styles from './shell.module.css';

/** The header: the path to the page and its title, the view tabs, the diagnostic count and the compare button. */
export function PageHeader({ crumbs, title, children }: { crumbs: string[]; title: ReactNode; children: ReactNode }) {
  return (
    <div className={styles.header}>
      <div className={styles.heading}>
        <p className="t-micro c-faint">{crumbs.join(' / ')}</p>
        <h1 className={styles.title}>{title}</h1>
      </div>
      {children}
    </div>
  );
}
