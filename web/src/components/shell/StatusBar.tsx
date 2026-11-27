import { GitCommitHorizontal, TriangleAlert } from '../marks/icons';
import { ABOUT, APP, CONTROLS, MARKS } from '../../terms';
import { Badge } from '../marks/Badge';
import { InfoHint } from '../../ui/hint';
import styles from './shell.module.css';
import { Icon } from '../../ui/icons';

/** The status bar: the branch and commit the snapshot was read at, the diagnostic count and the legend of the dots. */
export function StatusBar({ branch, commit, dirty, diagnostics }: { branch?: string; commit: string; dirty: boolean; diagnostics: number }) {
  return (
    <footer className={styles.statusBar}>
      <Icon glyph={GitCommitHorizontal} size="small" className="c-faint" />
      <span className="t-mono c-faint">{branch === undefined ? commit : `${branch}@${commit}`}</span>
      {dirty && <Badge tone="caution">{APP.dirty}</Badge>}
      <span className={styles.gap} />
      <span className={styles.statusCount} role="img" aria-label={CONTROLS.diagnostics}>
        <Icon glyph={TriangleAlert} size="small" />
        <span className="t-mono">{diagnostics}</span>
      </span>
      <span className={styles.gap} />
      <InfoHint subject={MARKS.status}>{ABOUT.statusLegend}</InfoHint>
    </footer>
  );
}
