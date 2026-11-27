import { digestOf, findEdge, has, plain, raw, withStatus } from './graph';
import { ADDED_ATTRS, ADDED_EDGES, HEAD, LINES_CHANGED, REVISIONS } from './shopChecks';
import type { Comparison, EdgeChange, Node, NodeChange, Revision } from './schemaTypes';

/* The base revisions the compare button picks, and how their snapshots differ from the served one (`unicode diff`). */

/**
 * The revision a `--rev` names: a commit of the branch by its full or abbreviated hash, the branch or HEAD, or one of
 * them followed by `~n`. Anything else git could not read.
 */
export function resolveRevision(rev: string): Revision | undefined {
  const ancestry = /^(?:main|HEAD)(?:~(\d+))?$/.exec(rev);
  if (ancestry) {
    return REVISIONS[Number(ancestry[1] ?? 0)];
  }
  return rev.length >= 4 ? REVISIONS.find(revision => revision.commit.startsWith(rev)) : undefined;
}

/** Whether the base is older than the head, and so lacks the head's change. */
function precedesHead(base: Revision): boolean {
  return base.commit !== HEAD.commit;
}

/** A node as the base revision's snapshot holds it: without the attribute values the head's change added. */
function beforeOf(id: string): Node {
  const node = raw(id);
  const added = ADDED_ATTRS[id] ?? {};
  const attrs = Object.fromEntries(Object.entries(node.attrs).map(([key, values]) => [key, values.filter(value => !(added[key] ?? []).includes(value))]));
  const before = { ...node, attrs };
  return { ...plain(id), attrs, digest: digestOf(before) };
}

/** How one node differs between the base and the served snapshot. */
export function nodeChangeOf(id: string, base: Revision): NodeChange {
  const after = plain(id);
  const changed = precedesHead(base) && (ADDED_ATTRS[id] !== undefined || LINES_CHANGED[id] !== undefined);
  if (!changed) {
    return { id, change: 'unchanged', before: after, after, attrs: [] };
  }
  const attrs = Object.entries(ADDED_ATTRS[id] ?? {}).map(([key, added]) => ({ key, removed: [], added }));
  const lines = LINES_CHANGED[id];
  return { id, change: 'modified', before: beforeOf(id), after, attrs, ...(lines !== undefined ? { linesChanged: lines } : {}) };
}

/** What differs between the base revision's snapshot and the served one. */
export function comparisonWith(base: Revision): Comparison {
  const ids = precedesHead(base) ? [...new Set([...Object.keys(ADDED_ATTRS), ...Object.keys(LINES_CHANGED)])].filter(has).sort() : [];
  const edges: EdgeChange[] = precedesHead(base)
    ? ADDED_EDGES.map(each => {
      const after = findEdge(each.kind, each.from, each.to);
      return { ...each, change: 'added' as const, ...(after ? { after: withStatus(after) } : {}) };
    })
    : [];
  return { base, head: HEAD, nodes: ids.map(id => nodeChangeOf(id, base)), edges };
}
