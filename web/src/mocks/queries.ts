import { CODE_SOURCES } from './codeSources';
import { declarationOf } from './excerpts';
import {
  ancestorsOf, childrenOf, diagnosticsAbout, edgeStatusOf, EDGES, findEdge, flowOf, full, has, incoming, inDeclarationOrder, neighbor, NODES, outgoing,
  parentOf, plain, raw, realizersOf, sortDiagnostics, statusOf, stepsOf, SYSTEM_ID, withStatus,
} from './graph';
import { BUILT_AT, DIAGNOSTICS, HEAD, SNAPSHOT_PATH } from './shopChecks';
import { edgeKey, type Affected, type EdgeEntity, type EdgeKind, type Neighbor, type Node, type RawEdge, type Slice, type Snapshot, type TreeNode, type ViewRequest } from './schemaTypes';
import { focusOf, NotFound } from './viewKit';

/* The interface's other answers: the snapshot, the design tree, an edge's preview, search, slices, impact and checks. */

/** Every declared item's and depends edge's status, by identity or by `kind from to`. */
export function statuses(): Record<string, NonNullable<Node['status']>> {
  const byId: Record<string, NonNullable<Node['status']>> = {};
  for (const node of NODES) {
    const status = statusOf(node.id);
    if (status) {
      byId[node.id] = status;
    }
  }
  for (const each of EDGES) {
    const status = edgeStatusOf(each);
    if (status) {
      byId[edgeKey(each)] = status;
    }
  }
  return byId;
}

export function snapshot(): Snapshot {
  const counts: Record<string, number> = {};
  for (const status of Object.values(statuses())) {
    counts[status] = (counts[status] ?? 0) + 1;
  }
  return {
    system: full(SYSTEM_ID),
    head: HEAD,
    path: SNAPSHOT_PATH,
    builtAt: BUILT_AT,
    counts: { nodes: NODES.length, edges: EDGES.length, diagnostics: DIAGNOSTICS.length, statuses: counts },
  };
}

/** The order of the system's groups in the tree: core, then support containers, external systems, persons, utility containers. */
function groupOf(id: string): number {
  const node = raw(id);
  if (node.kind === 'actor') {
    return node.form === 'external' ? 2 : 3;
  }
  return { core: 0, support: 1, utility: 4 }[focusOf(id)] ?? 1;
}

/** The design tree: containers by group and name; inside, what is declared in its order; steps and cases left out. */
export function tree(): TreeNode {
  const branch = (id: string): TreeNode => {
    let children = childrenOf(id).filter(child => raw(child).layer === 'design' && !['step', 'case'].includes(raw(child).kind));
    if (id === SYSTEM_ID) {
      children = [...children].sort((a, b) => groupOf(a) - groupOf(b) || raw(a).title.localeCompare(raw(b).title));
    }
    return { node: plain(id), children: children.map(branch) };
  };
  return branch(SYSTEM_ID);
}

/** The functions and files a depends edge's attrs name, and the detail functions a reaches edge passes through. */
function namedBy(each: RawEdge): string[] {
  const named = [...(each.attrs.via ?? []), ...(each.attrs.imports ?? []), ...(each.attrs.calls ?? [])].flatMap(entry => entry.split(' '));
  return [...new Set(named)].filter(has);
}

/** The preview of an edge a view draws. */
export function edgeEntity(kind: EdgeKind, from: string, to: string): EdgeEntity {
  const each = findEdge(kind, from, to);
  if (!each) {
    throw new NotFound(`the snapshot has no edge ${kind} ${from} ${to}`);
  }
  const related = incoming(to, 'traces').map(trace => trace.from);
  const views: ViewRequest[] = [
    ...(kind === 'reaches' ? [{ kind: 'calls' as const, input: plain(from), expand: [`reaches ${from} ${to}`] }] : []),
    ...(kind === 'depends' ? [{ kind: 'architecture' as const, input: plain(parentOf(from) ?? SYSTEM_ID), expand: [], select: edgeKey(each) }] : []),
    ...related.map(step => ({ kind: 'sequence' as const, input: plain(flowOf(step)), expand: [], select: step })),
  ];
  return {
    edge: withStatus(each),
    fromNode: full(from),
    toNode: full(to),
    nodes: namedBy(each).map(plain),
    steps: related.map(full),
    diagnostics: diagnosticsAbout(new Set([edgeKey(each)])),
    views,
  };
}

/** The nodes whose identity, title or summary contains the text, sorted by identity. */
export function search(text: string, kind: Node['kind'] | undefined, limit: number): Node[] {
  const needle = text.toLowerCase();
  return NODES
    .filter(node => (kind === undefined || node.kind === kind) && [node.id, node.title, node.summary].some(field => field.toLowerCase().includes(needle)))
    .map(node => node.id)
    .sort()
    .slice(0, limit)
    .map(full);
}

/** What the slice of a node adds by its kind (docs/design.md §7.3). */
function extrasOf(id: string): string[] {
  const node = raw(id);
  if (node.kind === 'op' && node.layer === 'design') {
    const owner = parentOf(id);
    const ownerOps = owner === undefined ? [] : childrenOf(owner).filter(child => raw(child).kind === 'op');
    return [
      ...(owner !== undefined && raw(owner).kind === 'type' ? [owner] : []), ...ownerOps,
      ...outgoing(id, 'subscribes').map(each => each.to), ...incoming(id, 'about').map(each => each.from),
      ...incoming(id, 'calls').map(each => each.from).filter(from => raw(from).kind === 'step'), ...realizersOf(id),
    ];
  }
  if (node.kind === 'property' || node.kind === 'flow') {
    const types: string[] = [];
    const pending = outgoing(id, 'refers').map(each => each.to);
    while (pending.length > 0) {
      const type = pending.shift() ?? '';
      if (!types.includes(type) && raw(type).kind === 'type') {
        types.push(type);
        pending.push(...outgoing(type, 'refers').map(each => each.to));
      }
    }
    const about = outgoing(id, 'about').map(each => each.to);
    const cases = childrenOf(id).filter(child => raw(child).kind === 'case');
    const predicate = incoming(id, 'checks').map(each => each.from);
    const flowSteps = node.kind === 'flow' ? stepsOf(id) : [];
    const traced = flowSteps.flatMap(step => outgoing(step, 'traces').map(each => each.to));
    return [...about, ...types, ...cases, ...predicate, ...flowSteps, ...traced];
  }
  return [];
}

/** The least context that answers a question about the nodes (`unicode slice`). */
export function slice(ids: string[], expand: string[], withCode: boolean): Slice {
  for (const id of ids) {
    if (!has(id)) {
      throw new NotFound(`the snapshot has no node ${id}`);
    }
  }
  const ancestors = [...new Map(ids.flatMap(ancestorsOf).map(node => [node.id, node])).values()].filter(node => !ids.includes(node.id));
  const neighbors: Neighbor[] = [];
  const details: Record<string, number> = {};
  for (const id of ids) {
    const edges = [...outgoing(id), ...incoming(id)].filter(each => each.kind !== 'contains' || raw(each.from).kind !== 'step');
    for (const each of edges) {
      const other = each.from === id ? each.to : each.from;
      if (raw(other).salient || expand.includes(other)) {
        neighbors.push(neighbor(id, each, other));
      } else {
        details[id] = (details[id] ?? 0) + 1;
      }
    }
  }
  neighbors.sort((a, b) => a.edge.kind.localeCompare(b.edge.kind) || a.edge.from.localeCompare(b.edge.from) || a.edge.to.localeCompare(b.edge.to));
  const extras = [...new Set(ids.flatMap(extrasOf))].filter(id => !ids.includes(id));
  const listed = new Set([...ids, ...ancestors.map(node => node.id), ...neighbors.map(each => each.node.id), ...extras]);
  const edges = EDGES.filter(each => each.kind === 'contains' && listed.has(each.from) && listed.has(each.to)).map(withStatus);
  const code: Record<string, string> = {};
  if (withCode) {
    for (const id of listed) {
      const node = raw(id);
      if (node.layer === 'code' && (node.kind === 'type' || node.kind === 'op') && CODE_SOURCES[node.origin.path] !== undefined) {
        code[id] = declarationOf(id).text;
      }
    }
  }
  return {
    focus: ids.map(full), ancestors, neighbors, extras: inDeclarationOrder(extras).map(full), details, edges,
    ...(withCode ? { code } : {}),
  };
}

/** The edge kinds a change travels back along (docs/design.md §7.4). */
const IMPACT_KINDS: EdgeKind[] = ['calls', 'reaches', 'refers', 'subscribes', 'emits', 'realizes', 'traces', 'exercises', 'depends', 'about', 'checks'];

/** The salient nodes that reach the changed ones along the impact edges, each with a shortest path, grouped by kind. */
export function impact(ids: string[]): Affected[] {
  for (const id of ids) {
    if (!has(id)) {
      throw new NotFound(`the snapshot has no node ${id}`);
    }
  }
  const paths = new Map<string, RawEdge[]>(ids.map(id => [id, []]));
  const frontier = [...ids];
  while (frontier.length > 0) {
    const current = frontier.shift() ?? '';
    for (const each of incoming(current).filter(edge => IMPACT_KINDS.includes(edge.kind))) {
      if (!paths.has(each.from)) {
        paths.set(each.from, [each, ...(paths.get(current) ?? [])]);
        frontier.push(each.from);
      }
    }
  }
  return [...paths]
    .filter(([id]) => !ids.includes(id) && raw(id).salient)
    .sort(([a], [b]) => raw(a).kind.localeCompare(raw(b).kind) || a.localeCompare(b))
    .map(([id, path]) => ({ node: plain(id), path: path.map(withStatus) }));
}

export function diagnostics() {
  return { diagnostics: sortDiagnostics(DIAGNOSTICS), statuses: statuses() };
}

/** Several nodes in the order asked, leaving out those the snapshot no longer has. */
export function nodesById(ids: string[]): Node[] {
  return ids.filter(has).map(full);
}

/** One node with the edges that leave it and reach it, sorted by kind, start and end. */
export function nodeView(id: string) {
  const sorted = (edges: RawEdge[]) => [...edges].sort((a, b) => a.kind.localeCompare(b.kind) || a.from.localeCompare(b.from) || a.to.localeCompare(b.to)).map(withStatus);
  return { node: full(id), outgoing: sorted(outgoing(id)), incoming: sorted(incoming(id)) };
}
