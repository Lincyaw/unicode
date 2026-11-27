import * as Tooltip from '@radix-ui/react-tooltip';
import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import type { Diagnostic, ViewNode } from '../../api/types';
import { CONTROLS, SECTIONS, STATUS_NAMES } from '../../terms';
import hintStyles from '../../ui/hint.module.css';
import { Badge } from '../marks/Badge';
import { useLinks } from '../marks/links';
import { StatusDot } from '../marks/StatusDot';
import styles from './views.module.css';

/**
 * A step's label on a flow's drawing: its index, status and what it does. It previews the step on a click and enters
 * its page on a double click; a step with diagnostics opens a card with them while the pointer rests on it.
 */
export function StepLabel({ step, diagnostics, children, className }: { step: ViewNode; diagnostics: Diagnostic[]; children: ReactNode; className?: string }) {
  const links = useLinks();
  const label = (
    <button
      type="button" className={clsx(styles.stepLabel, className)} aria-pressed={links.selected === step.id}
      onClick={() => links.select(step.id)} onDoubleClick={() => links.open(step.id)}
    >
      <span className={styles.stepIndex}>{step.title}</span>
      <StatusDot status={step.status} />
      {children}
    </button>
  );
  if (diagnostics.length === 0) {
    return label;
  }
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{label}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className={clsx(hintStyles.popover, styles.diagnosticCard)} side="bottom" align="start" sideOffset={8} collisionPadding={12}>
          <span className={styles.diagnosticHead}>
            <span className="t-mono-strong">{step.id}</span>
            <Badge tone="neutral">{step.form}</Badge>
          </span>
          {diagnostics.map(diagnostic => (
            <span key={diagnostic.code} className={styles.diagnosticBody}>
              <span className={styles.diagnosticCode}>
                <StatusDot status={step.status} />
                {step.status && STATUS_NAMES[step.status]} · {diagnostic.code}
              </span>
              <span>{diagnostic.message}</span>
              <span>{SECTIONS.fix}{CONTROLS.colon}{diagnostic.fix}</span>
            </span>
          ))}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}
