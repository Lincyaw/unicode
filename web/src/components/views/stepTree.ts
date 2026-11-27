import type { View, ViewNode } from '../../api/types';

/*
 * A flow's steps as the sequence and activity views walk them: the tree the contains edges and each step's children
 * make, in the order of their indexes.
 */

export interface StepTree {
  roots: ViewNode[];
  children: (step: ViewNode) => ViewNode[];
  lifelines: ViewNode[];
  byId: ReadonlyMap<string, ViewNode>;
}

function indexOf(step: ViewNode): number {
  return Number(step.attrs.index?.[0] ?? step.id.split('#').at(-1) ?? 0);
}

export function stepTree(view: View): StepTree {
  const steps = view.nodes.filter(node => node.kind === 'step');
  const byId = new Map(view.nodes.map(node => [node.id, node]));
  const roots = steps.filter(step => step.step?.parent === undefined).sort((a, b) => indexOf(a) - indexOf(b));
  const children = (step: ViewNode) => (step.step?.children ?? []).flatMap(id => byId.get(id) ?? []).sort((a, b) => indexOf(a) - indexOf(b));
  // The lifelines are the participants, the nodes that send or receive a step, in the view's order; a view may also
  // give the flow itself and what the steps call or emit.
  const participants = new Set(steps.flatMap(step => [step.step?.sender?.id, step.step?.receiver?.id].flatMap(id => (id === undefined ? [] : [id]))));
  const lifelines = view.nodes.filter(node => node.kind !== 'step' && participants.has(node.id));
  return { roots, children, lifelines, byId };
}

/** Every step under a step, depth first. */
export function descendants(tree: StepTree, step: ViewNode): ViewNode[] {
  return tree.children(step).flatMap(child => [child, ...descendants(tree, child)]);
}

/** The diagnostics a view gives about a node. */
export function diagnosticsAbout(view: View, id: string): View['diagnostics'] {
  return view.diagnostics.filter(diagnostic => diagnostic.subject === id);
}
