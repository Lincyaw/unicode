import type { Node } from '../../api/types';
import { CASE_SOURCES, STATUS_NAMES } from '../../terms';
import { statusOf } from '../marks/format';

/* The words a page says about a node: where a case comes from, and a status worth naming. */

/** Where a case comes from: declared, recorded for coverage, or a counterexample. */
export function caseSource(node: Node): string {
  if (node.form === 'declared') {
    return CASE_SOURCES.declared;
  }
  return node.attrs.reason?.[0] === 'counterexample' ? CASE_SOURCES.counterexample : CASE_SOURCES.recorded;
}

/** The status word of a node that is not verified, for a tag beside it such as an edge's end; undefined when verified. */
export function statusWord(node: Node): string | undefined {
  const status = statusOf(node);
  return status === undefined || status === 'verified' ? undefined : STATUS_NAMES[status];
}
