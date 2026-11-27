import * as Tooltip from '@radix-ui/react-tooltip';
import { clsx } from 'clsx';
import { Info } from 'lucide-react';
import type { ReactElement, ReactNode } from 'react';
import { Icon } from './icons';
import styles from './hint.module.css';
import { UI_WORDS } from './words';

/*
 * Hints keep pages quiet: an explanation or the detail of a mark waits in a card that opens while the pointer rests on
 * its trigger or the trigger has focus, instead of standing on the page as small print.
 */

/** How long the pointer rests on a trigger before its hint opens, and how soon the next one opens while moving between triggers. */
export const HINT_DELAY_MS = 250;
export const HINT_SKIP_DELAY_MS = 400;

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
        <Tooltip.Content className={clsx(styles.popover, styles.card)} sideOffset={6} collisionPadding={12}>{content}</Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}

/** A small round mark that explains what stands next to it, `subject`; screen readers name it as that subject's explanation. */
export function InfoHint({ subject, children }: { subject: string; children: ReactNode }) {
  return (
    <Hint content={children}>
      <button type="button" className={styles.info} aria-label={UI_WORDS.about(subject)}>
        <Icon glyph={Info} size="tiny" />
      </button>
    </Hint>
  );
}
