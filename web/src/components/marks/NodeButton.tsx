import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { useNodeActions } from './links';
import styles from './marks.module.css';

/*
 * The click targets of a drawing (docs/design.md §7.5): one click on a node previews it, a double click or Enter enters
 * its page. A box on a canvas is not itself a control, so that the references it holds can be controls of their own
 * without nesting one control in another: the box's head is a cover button whose hit area stretches over the box, and
 * every other control in the box (a reference, a toggle, a hint) is lifted above the cover.
 */

/**
 * The head of a box as its cover: the button's hit area covers the nearest positioned box around it, so a click
 * anywhere on the box that no lifted control takes previews the box's node.
 */
export function CoverButton({ id, className, children }: { id: string; className?: string; children: ReactNode }) {
  return <button type="button" className={clsx(styles.cover, className)} {...useNodeActions(id)}>{children}</button>;
}

/** A reference inside a box to another node, lifted above the box's cover: it previews and enters that node. */
export function NodeRef({ id, className, children }: { id: string; className?: string; children: ReactNode }) {
  return <button type="button" className={clsx(styles.ref, className)} data-ref {...useNodeActions(id)}>{children}</button>;
}

/** Lifts the controls it holds, such as a hint or a toggle, above the cover of their box. */
export function Above({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={clsx(styles.above, className)} data-lifted>{children}</span>;
}
