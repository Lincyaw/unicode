import { GitCommitHorizontal, TriangleAlert } from 'lucide-react';
import { ABOUT, APP, CONTROLS } from '../../terms';
import { Badge } from '../marks/Badge';
import { ICON_SMALL, ICON_STROKE, InfoHint } from '../../ui/hint';
import styles from './shell.module.css';

/** The status bar: the branch and commit the snapshot was read at, the diagnostic count and the legend of the dots. */
export function StatusBar({ branch, commit, dirty, diagnostics }: { branch?: string; commit: string; dirty: boolean; diagnostics: number }) {
  return (
    <footer className={styles.statusBar}>
      <GitCommitHorizontal size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" className="c-faint" />
      <span className="t-mono c-faint">{branch === undefined ? commit : `${branch}@${commit}`}</span>
      {dirty && <Badge tone="caution">{APP.dirty}</Badge>}
      <span className={styles.gap} />
      <span className={styles.statusCount} role="img" aria-label={CONTROLS.diagnostics}>
        <TriangleAlert size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
        <span className="t-mono">{diagnostics}</span>
      </span>
      <span className={styles.gap} />
      <InfoHint label={CONTROLS.details}>{ABOUT.statusLegend}</InfoHint>
    </footer>
  );
}
