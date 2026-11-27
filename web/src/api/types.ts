import type { components } from './schema';

/* The interface's models (api/*.tsp), named as the viewer uses them. */

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
export type ClosedTab = Schemas['ClosedTab'];
export type ClassLayer = Schemas['ClassLayer'];
export type C4Level = Schemas['C4Level'];
export type Diagnostic = Schemas['Diagnostic'];
export type CheckReport = Schemas['CheckReport'];
export type Evidence = Schemas['Evidence'];
export type Review = Schemas['Review'];
export type DriverMode = Schemas['DriverMode'];
export type RunResult = Schemas['RunResult'];
export type Entity = Schemas['Entity'];
export type FlowSteps = Schemas['FlowSteps'];
export type EdgeEntity = Schemas['EdgeEntity'];
export type OpRow = Schemas['OpRow'];
export type CaseRow = Schemas['CaseRow'];
export type CodeExcerpt = Schemas['CodeExcerpt'];
export type NodeChange = Schemas['NodeChange'];
export type ChangeKind = Schemas['ChangeKind'];
export type Comparison = Schemas['Comparison'];
export type Revision = Schemas['Revision'];
export type Snapshot = Schemas['Snapshot'];
export type TreeNode = Schemas['TreeNode'];
export type Problem = Schemas['Problem'];

/** The identity of an edge, as diagnostics, statuses and the address write it: `kind from to`. */
export function edgeKey(edge: { kind: EdgeKind; from: string; to: string }): string {
  return `${edge.kind} ${edge.from} ${edge.to}`;
}
