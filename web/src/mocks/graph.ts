import { CODE_EDGES, CODE_NODES } from './shopCode';
import { COVERS, DIAGNOSTICS, EVIDENCE, STATUS_EXCEPTIONS } from './shopChecks';
import { DESIGN_EDGES, DESIGN_NODES, SYSTEM_ID, typesNamedIn } from './shopDesign';
import { edgeKey, type Diagnostic, type Edge, type EdgeKind, type Evidence, type Member, type Neighbor, type Node, type RawEdge, type RawNode, type Status, type StepInfo } from './schemaTypes';

/*
 * The served snapshot: the shop's nodes and edges, indexed, and the fields the server adds to them when it answers
 * (status, bindings, a step's place in its flow, a type's members compared with the code).
 */

export const NODES: RawNode[] = [...DESIGN_NODES, ...CODE_NODES];
export const EDGES: RawEdge[] = [...DESIGN_EDGES, ...CODE_EDGES, ...COVERS];

const BY_ID = new Map(NODES.map(node => [node.id, node]));
const OUTGOING = Map.groupBy(EDGES, each => each.from);
const INCOMING = Map.groupBy(EDGES, each => each.to);
const ORDER = new Map(NODES.map((node, index) => [node.id, index]));

export { SYSTEM_ID };

export function has(id: string): boolean {
  return BY_ID.has(id);
}

export function raw(id: string): RawNode {
  const node = BY_ID.get(id);
  if (!node) {
    throw new Error(`the snapshot has no node ${id}`);
  }
  return node;
}

/** The edges leaving a node, of one kind or of any. */
export function outgoing(id: string, kind?: EdgeKind): RawEdge[] {
  return (OUTGOING.get(id) ?? []).filter(each => kind === undefined || each.kind === kind);
}

/** The edges reaching a node, of one kind or of any. */
export function incoming(id: string, kind?: EdgeKind): RawEdge[] {
  return (INCOMING.get(id) ?? []).filter(each => kind === undefined || each.kind === kind);
}

export function findEdge(kind: EdgeKind, from: string, to: string): RawEdge | undefined {
  return outgoing(from, kind).find(each => each.to === to);
}

/** Identities in the order the snapshot declares them. */
export function inDeclarationOrder(ids: Iterable<string>): string[] {
  return [...ids].sort((a, b) => (ORDER.get(a) ?? 0) - (ORDER.get(b) ?? 0));
}

/** The node that contains a node along contains, in its layer. */
export function parentOf(id: string): string | undefined {
  return incoming(id, 'contains')[0]?.from;
}

/** What a node contains, in the order of declaration. */
export function childrenOf(id: string): string[] {
  return outgoing(id, 'contains').map(each => each.to);
}

/** The status `unicode check` gives a node: declared design items have one; persons and code elements do not. */
export function statusOf(id: string): Status | undefined {
  const node = raw(id);
  if (node.layer !== 'design' || (node.kind === 'actor' && node.form === 'person')) {
    return undefined;
  }
  return STATUS_EXCEPTIONS[id] ?? 'verified';
}

/** The status of an edge: only uses and depends edges carry one. */
export function edgeStatusOf(each: { kind: EdgeKind; from: string; to: string }): Status | undefined {
  if (each.kind !== 'depends' && each.kind !== 'uses') {
    return undefined;
  }
  return STATUS_EXCEPTIONS[edgeKey(each)] ?? 'verified';
}

export function withStatus(each: RawEdge): Edge {
  const status = edgeStatusOf(each);
  return { ...each, ...(status ? { status } : {}) };
}

/** A content hash in the shape of SHA-256: deterministic, so the same content always hashes alike. */
export function digestOf(node: Pick<RawNode, 'kind' | 'form' | 'summary' | 'attrs'>): string {
  const content = JSON.stringify([node.kind, node.form, node.summary, Object.entries(node.attrs).sort(([a], [b]) => a.localeCompare(b))]);
  let digest = '';
  for (let seed = 0; seed < 8; seed += 1) {
    let hash = 0x811c9dc5 ^ seed;
    for (let index = 0; index < content.length; index += 1) {
      hash = Math.imul(hash ^ content.charCodeAt(index), 0x01000193) >>> 0;
    }
    digest += hash.toString(16).padStart(8, '0');
  }
  return digest;
}

/** A node with its hash and status, without the fields that name other nodes. */
export function plain(id: string): Node {
  const node = raw(id);
  const status = statusOf(id);
  return { ...node, digest: digestOf(node), ...(status ? { status } : {}) };
}

/** A node as the interface answers it: with a code element's bindings and a step's place in its flow. */
export function full(id: string): Node {
  const node = plain(id);
  const bindings = outgoing(id).filter(each => each.kind === 'realizes' || each.kind === 'checks' || each.kind === 'generates');
  return {
    ...node,
    ...(node.layer === 'code' ? { bindings: bindings.map(each => neighbor(id, each, each.to)) } : {}),
    ...(node.kind === 'step' ? { step: stepInfoOf(id) } : {}),
  };
}

/** An edge with the node at its other end, seen from `focus`. */
export function neighbor(focus: string, each: RawEdge, other: string): Neighbor {
  return { focus, edge: withStatus(each), node: plain(other) };
}

/** The flow a step belongs to. */
export function flowOf(step: string): string {
  let current = parentOf(step);
  while (current !== undefined && raw(current).kind === 'step') {
    current = parentOf(current);
  }
  if (current === undefined) {
    throw new Error(`step ${step} is in no flow`);
  }
  return current;
}

/** What a step calls, emits or handles. */
export function targetOf(step: string): string | undefined {
  return outgoing(step).find(each => each.kind === 'calls' || each.kind === 'emits' || each.kind === 'handles')?.to;
}

export function senderOf(step: string): string | undefined {
  return outgoing(step, 'from')[0]?.to;
}

/** The owner of the operation a step calls: its type, container or external system. */
export function receiverOf(step: string): string | undefined {
  const called = outgoing(step, 'calls')[0]?.to;
  return called === undefined ? undefined : parentOf(called);
}

export function stepInfoOf(id: string): StepInfo {
  const parent = parentOf(id);
  const [sender, target, receiver] = [senderOf(id), targetOf(id), receiverOf(id)];
  return {
    flow: flowOf(id),
    ...(sender ? { sender: plain(sender) } : {}),
    ...(target ? { target: plain(target) } : {}),
    ...(receiver ? { receiver: plain(receiver) } : {}),
    ...(parent !== undefined && raw(parent).kind === 'step' ? { parent } : {}),
    children: childrenOf(id).filter(child => raw(child).kind === 'step'),
  };
}

/** The steps of a flow in the order of their indexes, nested ones included. */
export function stepsOf(flow: string): string[] {
  const steps: string[] = [];
  const visit = (id: string) => {
    for (const child of childrenOf(id).filter(each => raw(each).kind === 'step')) {
      steps.push(child);
      visit(child);
    }
  };
  visit(flow);
  return steps.sort((a, b) => Number(raw(a).attrs.index?.[0]) - Number(raw(b).attrs.index?.[0]));
}

/** The design container (component, system or external system) a design node is declared in, the node itself included. */
export function designContainerOf(id: string): string | undefined {
  let current: string | undefined = id;
  while (current !== undefined && raw(current).kind !== 'container' && raw(current).kind !== 'actor') {
    current = parentOf(current);
  }
  return current;
}

/** The file a code element is written in, the element itself when it is a file. */
export function fileOf(id: string): string | undefined {
  let current: string | undefined = id;
  while (current !== undefined && raw(current).form !== 'file') {
    current = parentOf(current);
  }
  return current;
}

/** The design container that owns a code element, through its file (D1); a package's, through its first file. */
export function ownerOf(id: string): string | undefined {
  const node = raw(id);
  const file = node.form === 'package' ? childrenOf(id)[0] : fileOf(id);
  return file === undefined ? undefined : incoming(file, 'owns')[0]?.from;
}

/** The design container a node belongs to, for either layer. */
export function homeOf(id: string): string | undefined {
  return raw(id).layer === 'design' ? designContainerOf(id) : ownerOf(id);
}

/**
 * The nodes that contain a node, the system first. A code element's chain passes through the design container that
 * owns its file, then the file and the types between.
 */
export function ancestorsOf(id: string): Node[] {
  const chain: string[] = [];
  if (raw(id).layer === 'design') {
    for (let current = parentOf(id); current !== undefined; current = parentOf(current)) {
      chain.unshift(current);
    }
  } else {
    for (let current = parentOf(id); current !== undefined && raw(current).form !== 'package'; current = parentOf(current)) {
      chain.unshift(current);
    }
    const owner = ownerOf(id);
    if (owner !== undefined && owner !== id) {
      chain.unshift(...ancestorsOf(owner).map(node => node.id), owner);
    }
  }
  return chain.map(plain);
}

/** The code elements that realize a design node. */
export function realizersOf(id: string): string[] {
  return incoming(id, 'realizes').map(each => each.from);
}

/** The design node a code element realizes or checks. */
export function boundTo(id: string): string | undefined {
  return outgoing(id).find(each => each.kind === 'realizes' || each.kind === 'checks')?.to;
}

/** "name: type" pairs, by name. */
function namedTypes(entries: string[] | undefined): Map<string, string> {
  return new Map((entries ?? []).map(entry => {
    const colon = entry.indexOf(':');
    return [entry.slice(0, colon).trim(), entry.slice(colon + 1).trim()];
  }));
}

/** A design type's fields and enum values, compared with the code type that realizes it (rule 18). */
export function membersOf(typeId: string): Member[] {
  const node = raw(typeId);
  const code = realizersOf(typeId)[0];
  const codeNode = code === undefined ? undefined : raw(code);
  const codeFields = namedTypes(codeNode?.attrs.fields);
  const members: Member[] = [];
  for (const [name, type] of namedTypes(node.attrs.fields)) {
    const codeType = codeFields.get(name);
    const status: Status | undefined = codeNode === undefined ? undefined : codeType === undefined ? 'missing' : codeType === type ? 'verified' : 'contradicted';
    members.push({
      kind: 'field', name, type, ...(codeType !== undefined ? { codeType } : {}), inDesign: true, inCode: codeType !== undefined,
      refers: typesNamedIn(type), ...(status ? { status } : {}),
    });
  }
  const designValues = node.attrs.values ?? [];
  const codeValues = codeNode?.attrs.values ?? [];
  for (const value of designValues) {
    const inCode = codeValues.includes(value);
    members.push({ kind: 'value', name: value, inDesign: true, inCode, refers: [], ...(codeNode ? { status: inCode ? 'verified' : 'missing' } : {}) });
  }
  for (const value of codeValues.filter(each => !designValues.includes(each))) {
    members.push({ kind: 'value', name: value, inDesign: false, inCode: true, refers: [], status: 'contradicted' });
  }
  return members;
}

export function evidenceOf(id: string): Evidence | undefined {
  return EVIDENCE[id];
}

/** The diagnostics whose subject is among these, sorted by file, line, column and code. */
export function diagnosticsAbout(subjects: Set<string>): Diagnostic[] {
  return sortDiagnostics(DIAGNOSTICS.filter(each => subjects.has(each.subject)));
}

export function sortDiagnostics(diagnostics: Diagnostic[]): Diagnostic[] {
  return [...diagnostics].sort((a, b) => a.path.localeCompare(b.path) || a.line - b.line || a.column - b.column || a.code.localeCompare(b.code));
}

/** The subjects a set of nodes and edges stands for: the nodes' identities and the edges' `kind from to`. */
export function subjectsOf(nodes: Iterable<string>, edges: Iterable<{ kind: EdgeKind; from: string; to: string }>): Set<string> {
  return new Set([...nodes, ...[...edges].map(edgeKey)]);
}

/** The salient functions a function calls directly or reaches through details, with the edge. */
export function calleesOf(fn: string): RawEdge[] {
  return outgoing(fn).filter(each => (each.kind === 'calls' || each.kind === 'reaches') && raw(each.to).salient);
}

/** The functions that call a function, with the calls or reaches edge. */
export function callersOf(fn: string): RawEdge[] {
  return incoming(fn).filter(each => (each.kind === 'calls' || each.kind === 'reaches') && raw(each.from).kind === 'op' && raw(each.from).layer === 'code');
}

/** The detail functions a function calls directly. */
export function detailCalleesOf(fn: string): string[] {
  return outgoing(fn, 'calls').filter(each => !raw(each.to).salient).map(each => each.to);
}
