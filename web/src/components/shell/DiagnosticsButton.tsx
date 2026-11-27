import * as Popover from '@radix-ui/react-popover';
import { clsx } from 'clsx';
import { TriangleAlert } from '../marks/icons';
import { Link } from 'react-router';
import type { Diagnostic } from '../../api/types';
import { ABOUT, CONTROLS, SECTIONS } from '../../terms';
import { InfoHint } from '../../ui/hint';
import hintStyles from '../../ui/hint.module.css';
import styles from './shell.module.css';
import { Icon } from '../../ui/icons';

/** The header's diagnostic count, which opens the list of every diagnostic with its cause and fix. */
export function DiagnosticsButton({ diagnostics, subjectHref }: { diagnostics: Diagnostic[]; subjectHref: (subject: string) => string | undefined }) {
  return (
    <Popover.Root>
      <Popover.Trigger className={styles.diagnosticCount} aria-label={CONTROLS.diagnostics}>
        <Icon glyph={TriangleAlert} size="tiny" />
        {diagnostics.length}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className={clsx(hintStyles.popover, styles.panel)} align="start" sideOffset={8} collisionPadding={12} onOpenAutoFocus={event => event.preventDefault()}>
          <div className={styles.panelHead}>
            <span className="t-micro c-faint">{SECTIONS.diagnostics}</span>
            <InfoHint subject={SECTIONS.diagnostics}>{ABOUT.diagnostics}</InfoHint>
          </div>
          <ul className={styles.diagnosticList}>
            {diagnostics.map(diagnostic => {
              const href = subjectHref(diagnostic.subject);
              return (
                <li key={`${diagnostic.code} ${diagnostic.subject}`} className={styles.diagnostic}>
                  <span className={styles.diagnosticCode}>
                    <Icon glyph={TriangleAlert} size="tiny" />
                    {diagnostic.code}
                  </span>
                  {href === undefined ? <span className="t-mono">{diagnostic.subject}</span> : <Link className="t-mono" to={href}>{diagnostic.subject}</Link>}
                  <span className="t-small">{diagnostic.message}</span>
                  <span className="t-small c-muted">{SECTIONS.fix}{CONTROLS.colon}{diagnostic.fix}</span>
                  <span className="t-mono c-faint">{diagnostic.path}:{diagnostic.line}:{diagnostic.column}</span>
                </li>
              );
            })}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
