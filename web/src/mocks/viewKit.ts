import { ancestorsOf, childrenOf, diagnosticsAbout, evidenceOf, full, membersOf, raw, subjectsOf, withStatus } from './graph';
import type { C4Level, ClassLayer, EdgeLabel, RawEdge, View, ViewEdge, ViewKind, ViewNode } from './schemaTypes';

/* What every view shares: drawing a node or an edge, folding, and putting the answer together. */

/** The view does not take its input: the server answers 400 serve.not-applicable. */
export class NotApplicable extends Error {}

/** The request names an identity the snapshot does not have: 404 serve.not-found. */
export class NotFound extends Error {}

/** The labels of the DDD notation that apply to types (docs/model.md §5); an event is a domain event without one. */
export const DDD_TYPE_LABELS = ['aggregate', 'entity', 'value', 'domain-service', 'repository', 'factory'];

export function viewNode(id: string, extras: Partial<ViewNode> = {}): ViewNode {
  const evidence = evidenceOf(id);
  return { ...full(id), ...(evidence ? { evidence } : {}), ...extras };
}

export function viewEdge(each: RawEdge, extras: Partial<ViewEdge> = {}): ViewEdge {
  return { ...withStatus(each), ...extras };
}

/** A container's effective focus; a container without one (the system) is core. */
export function focusOf(container: string): string {
  return raw(container).attrs.focus?.[0] ?? 'core';
}

/** The operations of a type, each drawn as a row of its card. */
export function opsOf(type: string): string[] {
  return childrenOf(type).filter(child => raw(child).kind === 'op');
}

/** A type drawn as a card: its members compared with the code, and its operations as the card's contents. */
export function typeCard(type: string): ViewNode {
  return viewNode(type, { members: membersOf(type), contents: opsOf(type).map(op => viewNode(op)) });
}

/** A type a card lists by name only, such as one inside a folded container: no members, but its operations. */
export function typeName(type: string): ViewNode {
  return viewNode(type, { contents: opsOf(type).map(op => viewNode(op)) });
}

/** Every identity a view node stands for: itself and, recursively, its contents. */
function identitiesOf(nodes: ViewNode[]): string[] {
  return nodes.flatMap(node => [node.id, ...identitiesOf(node.contents ?? [])]);
}

export interface Drawing {
  nodes: ViewNode[];
  edges: ViewEdge[];
  folded?: string[];
  hidden?: number;
  layer?: ClassLayer;
  level?: C4Level;
}

/** A view's answer: the drawing, its input with the input's ancestors, and the diagnostics of what it draws. */
export function assembleView(kind: ViewKind, input: string, expand: string[], drawing: Drawing): Omit<View, 'tabs'> {
  return {
    kind,
    input: full(input),
    ancestors: ancestorsOf(input),
    expand,
    ...(drawing.layer ? { layer: drawing.layer } : {}),
    ...(drawing.level ? { level: drawing.level } : {}),
    nodes: drawing.nodes,
    edges: drawing.edges,
    folded: drawing.folded ?? [],
    hidden: drawing.hidden ?? 0,
    diagnostics: diagnosticsAbout(subjectsOf(identitiesOf(drawing.nodes), drawing.edges)),
  };
}

/** An edge's labels as unicode serve gives them: each operation once, sorted by its text, then by its identity. */
export function sortedLabels(labels: EdgeLabel[]): EdgeLabel[] {
  const order = (a: string, b: string) => (a < b ? -1 : Number(a > b));
  const unique = new Map(labels.map(label => [`${label.text}\u0000${label.op}`, label]));
  return [...unique.values()].sort((a, b) => order(a.text, b.text) || order(a.op, b.op));
}

/** Edges that stand for several edges of the graph, one per drawn end and folded container. */
export class Aggregates {
  private readonly byKey = new Map<string, ViewEdge>();

  add(each: RawEdge, from: string, to: string, labels: EdgeLabel[] = []) {
    const key = `${each.kind} ${from} ${to}`;
    const existing = this.byKey.get(key);
    if (existing) {
      existing.count = (existing.count ?? 0) + 1;
      const merged = sortedLabels([...(existing.labels ?? []), ...labels]);
      if (merged.length > 0) {
        existing.labels = merged;
      }
      return;
    }
    this.byKey.set(key, { kind: each.kind, from, to, source: 'derived', origin: each.origin, attrs: {}, count: 1, ...(labels.length > 0 ? { labels: sortedLabels(labels) } : {}) });
  }

  edges(): ViewEdge[] {
    return [...this.byKey.values()];
  }
}
