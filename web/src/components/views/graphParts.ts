import type { View, ViewEdge, ViewNode } from '../../api/types';
import { edgeKey } from '../../api/types';
import type { LineTone } from '../canvas/Edges';

/*
 * What the node-and-edge views share: an edge drawn between the boxes that stand for its ends (an operation's edge
 * leaves from its type's card; a node inside a folded container is drawn as the container), and the tone of its line.
 */

/** The drawn box that stands for an identity: itself, or the nearest drawn box whose identity it extends. */
export function boxFor(id: string, drawn: ReadonlySet<string>): string | undefined {
  if (drawn.has(id)) {
    return id;
  }
  let at = id.lastIndexOf('.');
  while (at > 0) {
    const prefix = id.slice(0, at);
    if (drawn.has(prefix)) {
      return prefix;
    }
    at = prefix.lastIndexOf('.');
  }
  return undefined;
}

/** An edge between two drawn boxes, with the edge it draws. */
export interface BoxEdge {
  id: string;
  from: string;
  to: string;
  edge: ViewEdge;
}

/** The view's edges between drawn boxes, one per pair and kind; an edge within one box is left out. */
export function boxEdges(edges: ViewEdge[], drawn: ReadonlySet<string>): BoxEdge[] {
  const seen = new Map<string, BoxEdge>();
  for (const edge of edges) {
    const from = boxFor(edge.from, drawn);
    const to = boxFor(edge.to, drawn);
    if (from === undefined || to === undefined || from === to) {
      continue;
    }
    const id = `${edge.kind} ${from} ${to}`;
    if (!seen.has(id)) {
      seen.set(id, { id, from, to, edge });
    }
  }
  return [...seen.values()];
}

/** The tone of an edge's line: a contradicted edge warns; otherwise by its kind. */
export function toneOf(edge: ViewEdge): LineTone {
  if (edge.status === 'contradicted') {
    return 'warning';
  }
  if (edge.kind === 'reaches') {
    return 'reach';
  }
  return edge.kind === 'emits' ? 'event' : 'plain';
}

/** Whether a refers edge comes from a field of its type (drawn solid) rather than an operation's signature (dashed). */
export function fromField(edge: ViewEdge, nodes: ReadonlyMap<string, ViewNode>): boolean {
  const source = nodes.get(edge.from);
  return source?.kind === 'type' && (source.members ?? []).some(member => member.refers.includes(edge.to) || member.refers.some(ref => edge.to === ref.split('.')[0]));
}

/** The size the page draws a tag at, as the kit's Tag draws one with an icon and a number of these digits. */
const TAG_HEIGHT = 24;
function tagWidth(digits: number): number {
  return 36 + 8 * digits;
}

/**
 * The size of the mark a call carries on its line, if any: how many detail functions a reaches edge passes, and how many
 * calls a merged edge stands for, each a tag, side by side. CallMark draws the mark at this size, and the layouts reserve
 * this room for it.
 */
export function markOf(edge: ViewEdge): { width: number; height: number } | undefined {
  const tags = [
    ...(edge.kind === 'reaches' ? [String(edge.attrs.via?.length ?? 0).length] : []),
    ...(edge.count === undefined ? [] : [String(edge.count).length]),
  ];
  return tags.length > 0 ? { width: tags.reduce((sum, digits) => sum + tagWidth(digits), 0) + 4 * (tags.length - 1), height: TAG_HEIGHT } : undefined;
}

/** The number of imports and calls an undeclared dependency stands for, as its badge counts them. */
export function evidenceCount(edge: ViewEdge): number {
  return (edge.attrs.imports?.length ?? 0) + (edge.attrs.calls?.length ?? 0);
}

/** Each external system that one component alone uses, to stand beside that component. */
export function externalsBeside(view: View): Map<string, string> {
  const actors = new Set(view.nodes.filter(node => node.kind === 'actor' && node.form === 'external').map(node => node.id));
  const users = Map.groupBy(view.edges.filter(edge => actors.has(edge.to)), edge => edge.to);
  return new Map([...users].flatMap(([actor, edges]) => (edges.length === 1 && edges[0] ? [[actor, edges[0].from]] : [])));
}

/**
 * What a component-level view draws as boxes and lines: every node but the system the view is of (whose containment
 * the drawing already is), and the dependencies between them, a declared use and the actual dependency of one pair
 * drawn once (the dependency, with its status; a use alone when the code does not depend).
 */
export function componentGraph(view: View): { nodes: View['nodes']; edges: ViewEdge[] } {
  const nodes = view.nodes.filter(node => !(node.kind === 'container' && node.form === 'system'));
  const drawn = new Set(nodes.map(node => node.id));
  const depends = new Set(view.edges.filter(edge => edge.kind === 'depends').map(edge => `${edge.from} ${edge.to}`));
  const edges = view.edges.filter(edge => drawn.has(edge.from) && drawn.has(edge.to) && (edge.kind === 'depends' || (edge.kind === 'uses' && !depends.has(`${edge.from} ${edge.to}`))));
  return { nodes, edges };
}

export { edgeKey };
