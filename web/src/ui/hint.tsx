import * as Tooltip from '@radix-ui/react-tooltip';
import { Info } from 'lucide-react';
import type { ReactElement, ReactNode } from 'react';
import styles from './hint.module.css';

/*
 * Hints keep pages quiet: an explanation or the detail of a mark waits in a card that opens while the pointer rests on
 * its trigger or the trigger has focus, instead of standing on the page as small print.
 */

/** How long the pointer rests on a trigger before its hint opens, and how soon the next one opens while moving between triggers. */
export const HINT_DELAY_MS = 250;
export const HINT_SKIP_DELAY_MS = 400;

/** The line icons' sizes and stroke: 16px in a row of text, 14px beside small text, 12px inside a tag; all stroked at 1.4. */
export const ICON_SIZE = 16;
export const ICON_SMALL = 14;
export const ICON_TINY = 12;
export const ICON_STROKE = 1.4;

/** Lets hints share their delays: once one has opened, the next opens at once. Wraps the whole site. */
export function HintProvider({ children }: { children: ReactNode }) {
  return <Tooltip.Provider delayDuration={HINT_DELAY_MS} skipDelayDuration={HINT_SKIP_DELAY_MS}>{children}</Tooltip.Provider>;
}

/** A card of detail beside its trigger, which must be an element that takes focus. */
export function Hint({ content, children }: { content: ReactNode; children: ReactElement }) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className={styles.card} sideOffset={6} collisionPadding={12}>{content}</Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/** A small ⓘ that explains what stands next to it; `label` names it for screen readers. */
export function InfoHint({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Hint content={children}>
      <button type="button" className={styles.info} aria-label={label}>
        <Info size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
      </button>
    </Hint>
  );
}
