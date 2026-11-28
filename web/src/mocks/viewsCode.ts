import { calleesOf, childrenOf, detailCalleesOf, findEdge, incoming, outgoing, ownerOf, raw, realizersOf } from './graph';
import type { RawEdge, ViewNode } from './schemaTypes';
import { Aggregates, focusOf, NotApplicable, viewEdge, viewNode, type Drawing } from './viewKit';

/* The views of the code: calls between salient functions, and a file with what calls into and out of it. */

/** How a function outside the view's home is drawn: as itself, inside a folded container, or not at all. */
type Placement = 'drawn' | 'folded' | 'hidden';

function placementOf(fn: string, home: string | undefined, expand: Set<string>): Placement {
  const owner = ownerOf(fn);
  if (owner === undefined || owner === home || expand.has(owner)) {
    return 'drawn';
  }
  const focus = focusOf(owner);
  return focus === 'core' ? 'drawn' : focus === 'support' ? 'folded' : 'hidden';
}

/** The identity an expansion of a reaches edge is written with. */
function reachesKey(each: RawEdge): string {
  return `reaches ${each.from} ${each.to}`;
}

/**
 * The functions realizing an operation (or the function given) and the salient functions they call or reach;
 * callees in a support container merge into the container. `expand` adds a salient function's direct detail callees,
 * or the detail functions a reaches edge passes through.
 */
export function calls(input: string, expand: string[]): Drawing {
  const node = raw(input);
  if (node.kind !== 'op') {
    throw new NotApplicable(`the calls view takes an operation or a function, and ${input} is a ${node.kind}`);
  }
  const roots = node.layer === 'design' ? realizersOf(input) : [input];
  const home = roots[0] === undefined ? undefined : ownerOf(roots[0]);
  const expanded = new Set(expand);
  const drawn: string[] = [...roots];
  const edges: RawEdge[] = [];
  const aggregates = new Aggregates();
  const foldedContents = new Map<string, string[]>();
  const hiddenContainers = new Set<string>();
  const draw = (id: string) => {
    if (!drawn.includes(id)) {
      drawn.push(id);
    }
  };
  for (const root of roots) {
    for (const each of calleesOf(root)) {
      const placement = placementOf(each.to, home, expanded);
      const owner = ownerOf(each.to) ?? '';
      if (placement === 'drawn') {
        draw(each.to);
        edges.push(each);
      } else if (placement === 'folded') {
        aggregates.add(each, each.from, owner);
        foldedContents.set(owner, [...new Set([...(foldedContents.get(owner) ?? []), each.to])]);
      } else {
        hiddenContainers.add(owner);
      }
    }
  }
  for (const item of expand) {
    const [verb, from, to] = item.split(' ');
    if (verb === 'reaches' && from !== undefined && to !== undefined) {
      // A reaches edge expands into the detail functions on its path and the calls between them.
      const path = [from, ...(findEdge('reaches', from, to)?.attrs.via ?? []), to];
      path.forEach(draw);
      for (const [index, fn] of path.slice(0, -1).entries()) {
        const call = findEdge('calls', fn, path[index + 1] ?? '');
        if (call) {
          edges.push(call);
        }
      }
    } else if (drawn.includes(item)) {
      for (const detail of detailCalleesOf(item)) {
        draw(detail);
        edges.push(...outgoing(item, 'calls').filter(each => each.to === detail));
      }
    }
  }
  // The calls between whatever else is drawn: from the detail functions an expansion added, and between salient callees.
  for (const fn of drawn.filter(id => !roots.includes(id))) {
    for (const each of outgoing(fn, 'calls').filter(edge => drawn.includes(edge.to))) {
      if (!edges.includes(each)) {
        edges.push(each);
      }
    }
  }
  const expandedReaches = new Set(expand);
  const shown = edges.filter(each => each.kind !== 'reaches' || !expandedReaches.has(reachesKey(each)));
  const folded = [...foldedContents.keys()];
  const nodes: ViewNode[] = [
    ...drawn.map(id => viewNode(id)),
    ...folded.map(container => viewNode(container, { contents: (foldedContents.get(container) ?? []).map(fn => viewNode(fn)) })),
  ];
  return { nodes, edges: [...shown.map(each => viewEdge(each)), ...aggregates.edges()], folded, hidden: hiddenContainers.size };
}

/** What a file or a type contains, at any depth. */
function within(id: string): string[] {
  return childrenOf(id).flatMap(child => [child, ...within(child)]);
}

/** How many detail functions a file or a type holds directly that the view does not draw. */
function detailCount(id: string, drawn: ReadonlySet<string>): number {
  return childrenOf(id).filter(child => raw(child).kind === 'op' && !raw(child).salient && !drawn.has(child)).length;
}

/** The detail functions reached from a function along calls (forward) or reaching it (backward), through details only. */
function detailClosure(start: string, forward: boolean): Set<string> {
  const found = new Set<string>();
  const queue = [start];
  for (let at = queue.shift(); at !== undefined; at = queue.shift()) {
    const next = forward ? outgoing(at, 'calls').map(each => each.to) : incoming(at, 'calls').map(each => each.from);
    for (const id of next.filter(item => raw(item).kind === 'op' && !raw(item).salient && !found.has(item))) {
      found.add(id);
      queue.push(id);
    }
  }
  return found;
}

/** What expanding x reveals, as internal/view's reveal: the direct detail callees of a drawn salient function, or the details on a drawn reaches edge's paths. */
function revealed(x: string, drawn: ReadonlySet<string>): { nodes: string[]; edges: RawEdge[] } {
  const [verb, from, to] = x.split(' ');
  if (verb === 'reaches' && from !== undefined && to !== undefined) {
    if (!drawn.has(from) || !drawn.has(to) || findEdge('reaches', from, to) === undefined) {
      return { nodes: [], edges: [] };
    }
    const after = detailClosure(from, true);
    const before = detailClosure(to, false);
    const on = (id: string) => after.has(id) && before.has(id);
    const nodes = [...after].filter(on).sort();
    const edges = [from, ...nodes].flatMap(id => outgoing(id, 'calls')).filter(each => (each.to === to || on(each.to)) && (on(each.from) || on(each.to)));
    return { nodes, edges };
  }
  if (!drawn.has(x) || raw(x).kind !== 'op' || !raw(x).salient) {
    return { nodes: [], edges: [] };
  }
  const nodes = detailCalleesOf(x);
  return { nodes, edges: outgoing(x, 'calls').filter(each => nodes.includes(each.to)) };
}

/**
 * A file (or a code type) as unicode serve gives it (internal/view's code selection and folding): the input and the
 * salient nodes it contains, each drawn on its own; the salient functions at the other end of a calls or reaches edge
 * of anything it contains, each drawn on its own, or merged into a folded container; the cases that cover it; and what
 * an expansion reveals. Of the calls and reaches edges between them it draws those with an end in the input, merging
 * the ones into a folded container; the input and its code types count their undrawn detail functions, and a folded
 * container lists its types.
 */
export function code(input: string, expand: string[]): Drawing {
  const node = raw(input);
  if (node.layer !== 'code' || !(node.form === 'file' || node.kind === 'type')) {
    throw new NotApplicable(`the code view takes a code file or a code type, and ${input} is a ${node.layer} ${node.kind}`);
  }
  const home = ownerOf(input);
  const expanded = new Set(expand);
  const contents = [input, ...within(input)];
  const inside = new Set(contents);
  const drawn = new Set(contents.filter(id => id === input || raw(id).salient));
  const into = new Map<string, string>();
  const hiddenContainers = new Set<string>();
  const place = (id: string) => {
    if (drawn.has(id) || into.has(id)) {
      return;
    }
    const placement = placementOf(id, home, expanded);
    const owner = ownerOf(id) ?? '';
    if (placement === 'drawn') {
      drawn.add(id);
    } else if (placement === 'folded') {
      into.set(id, owner);
    } else {
      hiddenContainers.add(owner);
    }
  };
  for (const id of contents) {
    [...outgoing(id), ...incoming(id)].filter(each => each.kind === 'calls' || each.kind === 'reaches')
      .map(each => (each.from === id ? each.to : each.from)).filter(other => raw(other).salient).forEach(place);
    incoming(id, 'covers').forEach(each => drawn.add(each.from));
  }
  const shown = expand.map(x => revealed(x, drawn));
  shown.flatMap(item => item.nodes).forEach(place);
  const revealedEdges = new Set(shown.flatMap(item => item.edges));
  const stand = (id: string) => (drawn.has(id) ? id : into.get(id) ?? '');
  const edges: RawEdge[] = [];
  const aggregates = new Aggregates();
  const kinds = ['contains', 'calls', 'reaches', 'covers'];
  for (const each of [...drawn, ...into.keys()].flatMap(id => outgoing(id))) {
    const [from, to] = [stand(each.from), stand(each.to)];
    const picked = kinds.includes(each.kind) && (!['calls', 'reaches'].includes(each.kind) || inside.has(each.from) || inside.has(each.to));
    if (from === '' || to === '' || (!picked && !revealedEdges.has(each)) || (from === to && (from !== each.from || to !== each.to))) {
      continue;
    }
    if (from === each.from && to === each.to) {
      edges.push(each);
    } else {
      aggregates.add(each, from, to);
    }
  }
  const foldedContainers = [...new Set(into.values())].sort();
  const typesIn = (container: string) => within(container).filter(id => raw(id).kind === 'type').sort();
  const counted = (id: string) => raw(id).layer === 'code' && (raw(id).kind === 'type' || raw(id).form === 'file');
  const nodes: ViewNode[] = [
    ...[...drawn].map(id => (counted(id) ? viewNode(id, { details: detailCount(id, drawn) }) : viewNode(id))),
    ...foldedContainers.map(container => viewNode(container, { contents: typesIn(container).map(type => viewNode(type)) })),
  ].sort((a, b) => (a.id < b.id ? -1 : 1));
  return { nodes, edges: [...edges.map(each => viewEdge(each)), ...aggregates.edges()], folded: foldedContainers, hidden: hiddenContainers.size };
}
