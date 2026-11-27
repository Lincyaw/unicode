import { calleesOf, callersOf, childrenOf, detailCalleesOf, fileOf, findEdge, incoming, outgoing, ownerOf, raw, realizersOf } from './graph';
import type { RawEdge, ViewEdge, ViewNode } from './schemaTypes';
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

/** The functions of a file or a type: its free functions and its types' methods, salient ones only. */
function functionsIn(id: string): string[] {
  return childrenOf(id).flatMap(child => (raw(child).kind === 'op' ? [child] : functionsIn(child))).filter(fn => raw(fn).salient);
}

/** How many detail functions a file or a type holds, nested ones included. */
function detailCount(id: string, drawn: Set<string>): number {
  return childrenOf(id).reduce((count, child) => count + (raw(child).kind === 'op' && !raw(child).salient && !drawn.has(child) ? 1 : 0) + detailCount(child, drawn), 0);
}

/**
 * A file (or a code type) with its salient types and functions, the salient functions that call into it and that it
 * calls or reaches, each in its own file, or merged into a folded container, and the cases that cover it.
 */
export function code(input: string, expand: string[]): Drawing {
  const node = raw(input);
  if (node.layer !== 'code' || !(node.form === 'file' || node.kind === 'type')) {
    throw new NotApplicable(`the code view takes a code file or a code type, and ${input} is a ${node.layer} ${node.kind}`);
  }
  const file = fileOf(input) ?? input;
  const home = ownerOf(input);
  const expanded = new Set(expand);
  const own = functionsIn(input);
  const details = new Set(expand.filter(item => own.includes(item)).flatMap(detailCalleesOf));
  const edges: RawEdge[] = [];
  const aggregates = new Aggregates();
  const others = new Map<string, string[]>();
  const foldedContents = new Map<string, string[]>();
  let hidden = 0;
  const place = (fn: string, each: RawEdge, from: string, to: string) => {
    if (own.includes(fn) || details.has(fn)) {
      edges.push(each);
      return;
    }
    const placement = placementOf(fn, home, expanded);
    const owner = ownerOf(fn) ?? '';
    if (placement === 'drawn') {
      const other = fileOf(fn) ?? fn;
      others.set(other, [...new Set([...(others.get(other) ?? []), fn])]);
      edges.push(each);
    } else if (placement === 'folded') {
      aggregates.add(each, from === fn ? owner : from, to === fn ? owner : to);
      foldedContents.set(owner, [...new Set([...(foldedContents.get(owner) ?? []), fn])]);
    } else {
      hidden += 1;
    }
  };
  for (const fn of own) {
    for (const each of calleesOf(fn)) {
      place(each.to, each, each.from, each.to);
    }
    for (const each of callersOf(fn).filter(edge => raw(edge.from).salient)) {
      place(each.from, each, each.from, each.to);
    }
    for (const detail of expand.includes(fn) ? detailCalleesOf(fn) : []) {
      edges.push(...outgoing(fn, 'calls').filter(each => each.to === detail));
    }
  }
  const covering = [...own, ...details].flatMap(fn => incoming(fn, 'covers'));
  const cases = [...new Set(covering.map(each => each.from))];
  const drawnDetails = details;
  // The input's card: the file, or the type alone, with its types and functions nested as the source nests them.
  const card = (id: string): ViewNode => viewNode(id, {
    contents: childrenOf(id).filter(child => raw(child).salient || drawnDetails.has(child)).map(child => (raw(child).kind === 'type' ? card(child) : viewNode(child))),
    ...(raw(id).form === 'file' || raw(id).kind === 'type' ? { details: detailCount(id, drawnDetails) } : {}),
  });
  const nodes: ViewNode[] = [
    node.kind === 'type' ? viewNode(file, { contents: [card(input)], details: detailCount(input, drawnDetails) }) : card(input),
    ...[...others].map(([other, fns]) => viewNode(other, { contents: fns.map(fn => viewNode(fn)), details: detailCount(other, new Set()) })),
    ...[...foldedContents].map(([container, fns]) => viewNode(container, { contents: fns.map(fn => viewNode(fn)) })),
    ...cases.map(id => viewNode(id)),
  ];
  const viewEdges: ViewEdge[] = [...edges.map(each => viewEdge(each)), ...aggregates.edges(), ...covering.map(each => viewEdge(each))];
  return { nodes, edges: viewEdges, folded: [...foldedContents.keys()], hidden };
}
