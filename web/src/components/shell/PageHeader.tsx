import { Fragment, type ReactNode } from 'react';
import { Link } from 'react-router';
import type { Crumb } from '../../ui/navigation';
import styles from './shell.module.css';

/**
 * The header: the path to the page and its title, the view tabs, the diagnostic count and the compare button. Each
 * node on the path is a link to its page.
 */
export function PageHeader({ crumbs, title, children }: { crumbs: Crumb[]; title: ReactNode; children: ReactNode }) {
  return (
    <div className={styles.header}>
      <div className={styles.heading}>
        <p className="t-micro c-faint">
          {crumbs.map((crumb, index) => (
            <Fragment key={`${index} ${crumb.label}`}>
              {index > 0 && ' / '}
              {crumb.to === undefined ? crumb.label : <Link className={styles.crumb} to={crumb.to}>{crumb.label}</Link>}
            </Fragment>
          ))}
        </p>
        <h1 className={styles.title}>{title}</h1>
      </div>
      {children}
    </div>
  );
}
