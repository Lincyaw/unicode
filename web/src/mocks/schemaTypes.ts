import type { components } from '../api/schema';

/* The interface's models, named as the mock uses them. */

type Schemas = components['schemas'];

export type Node = Schemas['Node'];
export type Edge = Schemas['Edge'];
export type EdgeKind = Schemas['EdgeKind'];
export type NodeKind = Schemas['NodeKind'];
export type Neighbor = Schemas['Neighbor'];
export type Position = Schemas['Position'];
export type Status = Schemas['Status'];
export type StepInfo = Schemas['StepInfo'];
export type Member = Schemas['Member'];
export type ViewNode = Schemas['ViewNode'];
export type ViewEdge = Schemas['ViewEdge'];
export type View = Schemas['View'];
export type ViewKind = Schemas['ViewKind'];
export type ViewRequest = Schemas['ViewRequest'];
export type ViewTab = Schemas['ViewTab'];
export type Diagnostic = Schemas['Diagnostic'];
export type Evidence = Schemas['Evidence'];
export type Entity = Schemas['Entity'];
export type FlowSteps = Schemas['FlowSteps'];
export type EdgeEntity = Schemas['EdgeEntity'];
export type CodeExcerpt = Schemas['CodeExcerpt'];
export type NodeChange = Schemas['NodeChange'];
export type EdgeChange = Schemas['EdgeChange'];
export type Comparison = Schemas['Comparison'];
export type Revision = Schemas['Revision'];
export type WorkingTree = Schemas['WorkingTree'];
export type Snapshot = Schemas['Snapshot'];
export type TreeNode = Schemas['TreeNode'];
export type Slice = Schemas['Slice'];
export type Affected = Schemas['Affected'];
export type Problem = Schemas['Problem'];
export type Review = Schemas['Review'];
export type ClassLayer = Schemas['ClassLayer'];
export type C4Level = Schemas['C4Level'];

/** A node as the fixture writes it: what the snapshot holds, before the check and the derived fields are added. */
export type RawNode = Omit<Node, 'digest' | 'status' | 'refers' | 'bindings' | 'step'>;

/** An edge as the fixture writes it, before the check gives it a status. */
export type RawEdge = Omit<Edge, 'status'>;

/** The identity of an edge, as diagnostics and statuses write it: `kind from to`. */
export function edgeKey(edge: { kind: EdgeKind; from: string; to: string }): string {
  return `${edge.kind} ${edge.from} ${edge.to}`;
}
