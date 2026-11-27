import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { MarkButton } from '../../ui/controls';
import { useNodeActions } from './links';
import styles from './marks.module.css';

/*
 * A box on a canvas is not itself a control, so that the references it holds can be controls of their own without
 * nesting one control in another (docs/design.md §7.5): the box's head is a cover button whose hit area stretches over
 * the box, and every other control in the box (a NodeRef, a toggle or a hint in an Above) is lifted above the cover.
 */

/**
 * The head of a box as its cover: the button's hit area covers the nearest positioned box around it, so a click
 * anywhere on the box that no lifted control takes previews the box's node, and a double click or Enter enters its page.
 */
export function CoverButton({ id, className, children }: { id: string; className?: string; children: ReactNode }) {
  return <MarkButton className={clsx(styles.cover, className)} {...useNodeActions(id)}>{children}</MarkButton>;
}
