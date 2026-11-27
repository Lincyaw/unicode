import { clsx } from 'clsx';
import { ABOUT, APP, CONTROLS, MARKS } from '../../terms';
import { Tag } from '../../ui/content';
import { InfoHint } from '../../ui/hint';
import { Icon } from '../../ui/icons';
import { GitCommitHorizontal, TriangleAlert } from '../marks/icons';
import styles from './shell.module.css';

/**
 * The status bar: the branch and commit the snapshot was read at, the diagnostic count, whose icon takes the failure
 * colour only when there is a diagnostic, and the legend of the dots.
 */
export function StatusBar({ branch, commit, dirty, diagnostics }: { branch?: string; commit: string; dirty: boolean; diagnostics: number }) {
  return (
    <footer className={styles.statusBar}>
      <Icon glyph={GitCommitHorizontal} size="small" className="c-muted" />
      <span className="t-mono c-muted">{branch === undefined ? commit : `${branch}@${commit}`}</span>
      {dirty && <Tag tone="caution">{APP.dirty}</Tag>}
      <span className={styles.gap} />
      <span className={styles.statusCount} role="img" aria-label={CONTROLS.diagnosticsCount(diagnostics)}>
        <Icon glyph={TriangleAlert} size="small" className={clsx(diagnostics > 0 && styles.alert)} />
        <span className="t-mono">{diagnostics}</span>
      </span>
      <span className={styles.gap} />
      <InfoHint subject={MARKS.status}>{ABOUT.statusLegend}</InfoHint>
    </footer>
  );
}
