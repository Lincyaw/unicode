import * as Popover from '@radix-ui/react-popover';
import { clsx } from 'clsx';
import { Link } from 'react-router';
import type { Diagnostic } from '../../api/types';
import { ABOUT, CONTROLS, SECTIONS } from '../../terms';
import { HintedHeading, Tag } from '../../ui/content';
import { Button } from '../../ui/controls';
import hintStyles from '../../ui/hint.module.css';
import { Icon } from '../../ui/icons';
import { TriangleAlert } from '../marks/icons';
import styles from './shell.module.css';

/**
 * The header's diagnostic count, a quiet capsule that opens the list of every diagnostic with its cause and fix. Its
 * icon takes the failure colour only when there is a diagnostic.
 */
export function DiagnosticsButton({ diagnostics, subjectHref }: { diagnostics: Diagnostic[]; subjectHref: (subject: string) => string | undefined }) {
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <Button variant="quiet" size="medium" label={CONTROLS.diagnosticsCount(diagnostics.length)}>
          <Icon glyph={TriangleAlert} size="small" className={clsx(diagnostics.length > 0 && styles.alert)} />
          <span className="t-numeric">{diagnostics.length}</span>
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className={clsx(hintStyles.popover, styles.panel)} align="start" sideOffset={8} collisionPadding={12} onOpenAutoFocus={event => event.preventDefault()}>
          <HintedHeading rank="h2" className="t-label" title={SECTIONS.diagnostics} about={ABOUT.diagnostics} />
          <ul className={styles.diagnosticList}>
            {diagnostics.map(diagnostic => {
              const href = subjectHref(diagnostic.subject);
              return (
                <li key={`${diagnostic.code} ${diagnostic.subject}`} className={styles.diagnostic}>
                  <Tag tone="warning"><Icon glyph={TriangleAlert} size="tiny" />{diagnostic.code}</Tag>
                  {href === undefined ? <span className="t-mono">{diagnostic.subject}</span> : <Link className="t-mono" to={href}>{diagnostic.subject}</Link>}
                  <span className="t-small">{diagnostic.message}</span>
                  <span className="t-small c-muted">{SECTIONS.fix}{CONTROLS.colon}{diagnostic.fix}</span>
                  <span className="t-mono c-muted">{diagnostic.path}:{diagnostic.line}:{diagnostic.column}</span>
                </li>
              );
            })}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
