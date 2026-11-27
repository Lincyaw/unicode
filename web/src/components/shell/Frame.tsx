import type { ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import styles from './shell.module.css';

/**
 * The viewer's frame: the top bar, the header, then the design tree, the canvas and the right pane side by side, and
 * the status bar. It fills the window; each of the three columns scrolls on its own.
 */
export function Frame({ topBar, header, tree, main, pane, statusBar }: {
  topBar: ReactNode; header: ReactNode; tree: ReactNode; main: ReactNode; pane: ReactNode; statusBar: ReactNode;
}) {
  return (
    <div className={styles.frame}>
      {topBar}
      {header}
      <div className={styles.body}>
        <nav className={styles.tree} aria-label={CONTROLS.tree}>{tree}</nav>
        <main className={styles.main}>{main}</main>
        <aside className={styles.pane} aria-label={CONTROLS.details}>{pane}</aside>
      </div>
      {statusBar}
    </div>
  );
}
