import type { FallbackProps } from 'react-error-boundary';
import { ApiError } from '../../api/errors';
import { CONTROLS, PROBLEM_WORDS, SECTIONS } from '../../terms';
import styles from './details.module.css';

/** A refused request, shown where its data would be: the code, the cause and the fix, as a diagnostic reads. */
export function ProblemNote({ error }: FallbackProps) {
  if (!(error instanceof ApiError)) {
    return (
      <div className={styles.problem} role="alert">
        <span className="t-micro c-faint">{PROBLEM_WORDS.failed}</span>
        <span className="t-mono c-bad">{error instanceof Error ? error.message : String(error)}</span>
      </div>
    );
  }
  return (
    <div className={styles.problem} role="alert">
      <span className="t-micro c-faint">{PROBLEM_WORDS.title}</span>
      <span className="t-mono c-bad">{error.problem.code}</span>
      <span className="t-small">{error.problem.message}</span>
      <span className="t-small c-muted">{SECTIONS.fix}{CONTROLS.colon}{error.problem.fix}</span>
    </div>
  );
}
