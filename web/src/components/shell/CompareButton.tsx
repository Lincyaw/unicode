import * as Popover from '@radix-ui/react-popover';
import { clsx } from 'clsx';
import { Check, GitCompareArrows } from 'lucide-react';
import { Link } from 'react-router';
import type { Comparison, Revision } from '../../api/types';
import { ABOUT, CHANGE_KINDS, CONTROLS } from '../../terms';
import { ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import hintStyles from '../../ui/hint.module.css';
import styles from './shell.module.css';

/**
 * The compare button: it names the base revision the pages compare with, and opens the recent commits to choose
 * another, with what differs from the chosen one.
 */
export function CompareButton({ base, revisions, comparison, baseHref, entityHref }: {
  base?: string;
  revisions: Revision[];
  comparison?: Comparison;
  baseHref: (base: string | undefined) => string;
  entityHref: (id: string) => string;
}) {
  const changed = comparison?.nodes.filter(change => change.change !== 'unchanged') ?? [];
  return (
    <Popover.Root>
      <Popover.Trigger className={styles.compare} aria-label={CONTROLS.compare}>
        <GitCompareArrows size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
        <span className="t-mono">{base ?? CONTROLS.compare}</span>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className={clsx(hintStyles.popover, styles.panel)} align="start" sideOffset={8} collisionPadding={12}>
          <div className={styles.panelHead}>
            <span className="t-micro c-faint">{CONTROLS.compare}</span>
            <InfoHint label={CONTROLS.compare}>{ABOUT.compare}</InfoHint>
          </div>
          <ul className={styles.revisions}>
            <li>
              <Link className={styles.revision} to={baseHref(undefined)} aria-current={base === undefined ? 'true' : undefined}>
                <span className={styles.revisionMark}>{base === undefined && <Check size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />}</span>
                <span className="t-small c-muted">{CONTROLS.compareNone}</span>
              </Link>
            </li>
            {revisions.map(revision => (
              <li key={revision.commit}>
                <Link className={styles.revision} to={baseHref(revision.short)} aria-current={base === revision.short ? 'true' : undefined}>
                  <span className={styles.revisionMark}>{base === revision.short && <Check size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />}</span>
                  <span className="t-mono">{revision.short}</span>
                  <span className={clsx(styles.revisionSubject, 't-small c-muted')}>{revision.subject}</span>
                </Link>
              </li>
            ))}
          </ul>
          {changed.length > 0 && (
            <ul className={styles.changes}>
              {changed.map(change => (
                <li key={change.id} className={styles.change}>
                  <span className="t-micro c-faint">{CHANGE_KINDS[change.change]}</span>
                  {change.change === 'removed' ? <span className="t-mono c-faint">{change.id}</span> : <Link className="t-mono" to={entityHref(change.id)}>{change.id}</Link>}
                </li>
              ))}
            </ul>
          )}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
