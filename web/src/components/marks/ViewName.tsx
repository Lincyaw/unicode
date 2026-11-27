import { NAME_SEPARATOR } from '../../terms';

/**
 * A view's name with its input's, as a title or a link to the view writes it. The separator is drawn but not read, so
 * a screen reader says the two names alone.
 */
export function ViewName({ view, input }: { view: string; input: string }) {
  return <>{view}<span aria-hidden="true">{NAME_SEPARATOR}</span>{input}</>;
}
