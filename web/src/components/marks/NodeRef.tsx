import { clsx } from 'clsx';
import type { ComponentProps } from 'react';
import { MarkButton } from '../../ui/controls';
import { useNodeActions } from './links';
import styles from './marks.module.css';

/**
 * A reference on a drawing to a node, such as a field's type inside a card, lifted above the cover of its box: a click
 * previews the node, a double click or Enter enters its page. Other props reach the button, so a hint can make it its
 * trigger.
 */
export function NodeRef({ id, className, children, ...rest }: { id: string } & ComponentProps<typeof MarkButton>) {
  return <MarkButton {...rest} className={clsx(styles.ref, className)} data-ref {...useNodeActions(id)}>{children}</MarkButton>;
}
