import { calleesOf, callersOf, childrenOf, detailCalleesOf, findEdge, incoming, outgoing, ownerOf, raw, realizersOf } from './graph';
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

/** How many detail functions a file or a type holds, nested ones included, that the view does not draw. */
function detailCount(id: string, drawn: Set<string>): number {
  return childrenOf(id).reduce((count, child) => count + (raw(child).kind === 'op' && !raw(child).salient && !drawn.has(child) ? 1 : 0) + detailCount(child, drawn), 0);
}

/**
 * A file (or a code type) as unicode serve gives it (internal/view's code selection): the input and the salient nodes
 * it contains, each drawn on its own with the contains edges between them; the salient functions that call into it and
 * that it calls or reaches, each drawn on its own, or merged into a folded container; and the cases that cover it. The
 * input and its code types count their undrawn detail functions; a folded container lists its types.
 */
export function code(input: string, expand: string[]): Drawing {
  const node = raw(input);
  if (node.layer !== 'code' || !(node.form === 'file' || node.kind === 'type')) {
    throw new NotApplicable(`the code view takes a code file or a code type, and ${input} is a ${node.layer} ${node.kind}`);
  }
  const home = ownerOf(input);
  const expanded = new Set(expand);
  const inside = within(input).filter(id => raw(id).salient);
  const drawn = new Set([input, ...inside]);
  const edges: RawEdge[] = inside.flatMap(id => incoming(id, 'contains').filter(each => drawn.has(each.from)));
  const aggregates = new Aggregates();
  const foldedContainers = new Set<string>();
  const hiddenContainers = new Set<string>();
  const place = (fn: string, each: RawEdge) => {
    const placement = drawn.has(fn) ? 'drawn' : placementOf(fn, home, expanded);
    const owner = ownerOf(fn) ?? '';
    if (placement === 'drawn') {
      drawn.add(fn);
      if (!edges.includes(each)) {
        edges.push(each);
      }
    } else if (placement === 'folded') {
      aggregates.add(each, each.from === fn ? owner : each.from, each.to === fn ? owner : each.to);
      foldedContainers.add(owner);
    } else {
      hiddenContainers.add(owner);
    }
  };
  for (const fn of inside.filter(id => raw(id).kind === 'op')) {
    calleesOf(fn).forEach(each => place(each.to, each));
    callersOf(fn).filter(each => raw(each.from).salient).forEach(each => place(each.from, each));
  }
  const covering = inside.flatMap(id => incoming(id, 'covers'));
  const typesIn = (container: string) => within(container).filter(id => raw(id).kind === 'type');
  const nodes: ViewNode[] = [
    ...[...drawn].map(id => (id === input || (raw(id).kind === 'type' && inside.includes(id)) ? viewNode(id, { details: detailCount(id, drawn) }) : viewNode(id))),
    ...[...foldedContainers].map(container => viewNode(container, { contents: typesIn(container).map(type => viewNode(type)) })),
    ...[...new Set(covering.map(each => each.from))].map(id => viewNode(id)),
  ].sort((a, b) => (a.id < b.id ? -1 : 1));
  return {
    nodes,
    edges: [...edges.map(each => viewEdge(each)), ...aggregates.edges(), ...covering.map(each => viewEdge(each))],
    folded: [...foldedContainers].sort(),
    hidden: hiddenContainers.size,
  };
}
