import { nodeChangeOf } from './compare';
import { declarationLine, declarationOf } from './excerpts';
import {
  ancestorsOf, calleesOf, callersOf, childrenOf, designContainerOf, detailCalleesOf, diagnosticsAbout, evidenceOf, findEdge, flowOf, full,
  incoming, membersOf, neighbor, outgoing, ownerOf, parentOf, plain, raw, realizersOf, senderOf, stepsOf, subjectsOf, SYSTEM_ID, targetOf,
} from './graph';
import type { Entity, FlowSteps, Neighbor, Node, RawEdge, Revision, ViewKind, ViewRequest } from './schemaTypes';
import { opsOf } from './viewKit';
import { callBefore } from './viewsFlow';

/* The page of one node (api/entities.tsp): the sections every node has, and the one of its kind. */

function unique(ids: string[]): string[] {
  return [...new Set(ids)];
}

/** Every design node under a node, the node excluded. */
function descendantsOf(id: string): string[] {
  return childrenOf(id).flatMap(child => [child, ...descendantsOf(child)]);
}

/** A node's operations: a type's own; a container's or an external system's, at any depth. */
function operationsOf(id: string): string[] {
  const node = raw(id);
  if (node.kind === 'op') {
    return [id];
  }
  return node.kind === 'type' ? opsOf(id) : descendantsOf(id).filter(each => raw(each).kind === 'op');
}

/** The flows it takes part in: the steps that call the node or its operations, call an operation whose signature refers to it, emit or handle it, or that it sends as a person. */
export function stepsTakingPart(id: string): string[] {
  const ops = raw(id).layer === 'design' ? operationsOf(id) : [];
  const referring = incoming(id, 'refers').map(each => each.from).filter(from => raw(from).kind === 'op');
  const steps = [
    ...[...ops, ...referring].flatMap(op => incoming(op, 'calls').map(each => each.from)),
    ...incoming(id, 'emits').map(each => each.from),
    ...incoming(id, 'handles').map(each => each.from),
    ...(raw(id).kind === 'actor' && raw(id).form === 'person' ? incoming(id, 'from').map(each => each.from) : []),
  ].filter(step => raw(step).kind === 'step');
  return unique(steps).sort((a, b) => flowOf(a).localeCompare(flowOf(b)) || Number(raw(a).attrs.index?.[0]) - Number(raw(b).attrs.index?.[0]));
}

/** The related properties: the properties about the node or its operations; a step's, about its flow or what it or an enclosing step calls. */
export function relatedProperties(id: string): string[] {
  const node = raw(id);
  let subjects: string[];
  if (node.kind === 'step') {
    const enclosing: string[] = [];
    for (let current: string | undefined = id; current !== undefined && raw(current).kind === 'step'; current = parentOf(current)) {
      enclosing.push(current);
    }
    subjects = [flowOf(id), ...enclosing.map(targetOf).filter((target): target is string => target !== undefined)];
  } else {
    subjects = [id, ...(node.kind === 'container' || node.kind === 'actor' ? [] : operationsOf(id))];
  }
  return unique(subjects.flatMap(subject => incoming(subject, 'about').map(each => each.from)));
}

function request(kind: ViewKind, input: string, extras: Partial<ViewRequest> = {}): ViewRequest {
  return { kind, input: plain(input), expand: [], ...extras };
}

/** Whether the system file declares a notation, so its labelled view exists. */
function hasNotation(notation: string): boolean {
  return (raw(SYSTEM_ID).attrs.notations ?? []).includes(notation);
}

/** The related views (api/entities.tsp): what takes the node as input, then the views of its container, flows, operations and properties. */
export function relatedViews(id: string): ViewRequest[] {
  const node = raw(id);
  const requests: ViewRequest[] = [];
  const add = (each: ViewRequest) => {
    if (!requests.some(other => other.kind === each.kind && other.input.id === each.input.id && other.select === each.select && other.expand.join() === each.expand.join())) {
      requests.push(each);
    }
  };
  if (node.layer === 'design') {
    if (node.kind === 'container') {
      add(request('architecture', id));
      if (hasNotation('ddd') && (id === SYSTEM_ID || (node.attrs.labels ?? []).includes('bounded-context'))) {
        add(request('ddd', id));
      }
      if (node.form === 'component') {
        add(request('class', id));
      }
      add(request('events', id));
      add(request('property', id));
      if (hasNotation('c4')) {
        add(request('c4', id));
      }
    } else if (node.kind === 'actor') {
      // An actor is no container: the system's views draw it, an external system in the architecture, both in C4.
      if (node.form === 'external') {
        add(request('architecture', SYSTEM_ID, { select: id }));
      }
      if (hasNotation('c4')) {
        add(request('c4', SYSTEM_ID, { select: id }));
      }
    } else if (node.kind === 'type') {
      add(request('class', id));
    } else if (node.kind === 'op') {
      add(request('calls', id));
    } else if (node.kind === 'flow') {
      add(request('sequence', id));
      add(request('activity', id));
    } else if (node.kind === 'property') {
      add(request('property', id));
    } else if (node.kind === 'step') {
      add(request('sequence', flowOf(id), { select: id }));
      add(request('activity', flowOf(id), { select: id }));
    } else if (node.kind === 'case' && raw(parentOf(id) ?? id).kind === 'property') {
      add(request('property', parentOf(id) ?? id, { select: id }));
    }
  } else if (node.form === 'file' || node.kind === 'type') {
    add(request('code', id));
  } else if (node.kind === 'op') {
    add(request('calls', id));
  }
  // The class, DDD and architecture views of its container: those that draw a type or an operation, and the
  // architecture view, which also draws flows, properties and the code's owners.
  const home = node.layer === 'design' ? designContainerOf(id) : ownerOf(id);
  const onCards = node.layer === 'design' && (node.kind === 'type' || node.kind === 'op');
  if (home !== undefined && home !== id && raw(home).kind === 'container' && raw(home).form === 'component' && node.kind !== 'step' && node.kind !== 'case') {
    if (onCards) {
      add(request('class', home, { select: id }));
      if (hasNotation('ddd') && (raw(home).attrs.labels ?? []).includes('bounded-context')) {
        add(request('ddd', home, { select: id }));
      }
    }
    add(request('architecture', home, { select: node.layer === 'design' ? id : home }));
  } else if (home !== undefined && home !== id && raw(home).kind === 'actor') {
    add(request('architecture', SYSTEM_ID, { select: home }));
  }
  // The sequence and activity views of the flows it takes part in, each with its first step selected.
  const design = node.layer === 'design' ? id : outgoing(id, 'realizes')[0]?.to;
  for (const step of design === undefined || node.kind === 'step' ? [] : stepsTakingPart(design)) {
    add(request('sequence', flowOf(step), { select: step }));
    add(request('activity', flowOf(step), { select: step }));
  }
  // The calls view of its operations, of the operation a step calls, or of what a case's property is about.
  const called = node.kind === 'step' ? targetOf(id) : undefined;
  const caseAbout = node.kind === 'case' ? outgoing(parentOf(id) ?? id, 'about').map(each => each.to).filter(about => raw(about).kind === 'op') : [];
  for (const op of node.kind === 'type' && node.layer === 'design' ? opsOf(id) : called !== undefined && raw(called).kind === 'op' ? [called] : caseAbout) {
    add(request('calls', op));
  }
  if (node.layer === 'code' && node.kind === 'op') {
    const realized = outgoing(id, 'realizes')[0]?.to;
    if (realized !== undefined) {
      add(request('calls', realized));
    }
  }
  // The property view of its properties.
  for (const each of design === undefined ? [] : relatedProperties(design)) {
    add(request('property', each));
  }
  return requests;
}

function steps(ids: string[]): Node[] {
  return ids.map(full);
}

/** Steps in flow order, grouped by their flow: each flow's node with its steps. */
function flowSteps(ids: string[]): FlowSteps[] {
  return [...Map.groupBy(ids, flowOf)].map(([flow, group]) => ({ flow: full(flow), steps: steps(group) }));
}

/** Uses and depends edges at a container, one per other end: a depends where there is one, else the uses. */
function dependencyEdges(edges: RawEdge[], otherEnd: (each: RawEdge) => string): RawEdge[] {
  const depends = edges.filter(each => each.kind === 'depends');
  const uses = edges.filter(each => each.kind === 'uses' && !depends.some(other => otherEnd(other) === otherEnd(each)));
  return [...depends, ...uses];
}

/** The calls edges into a step's traced function from the code of its enclosing call's operation or of its sender. */
function callSitesOf(step: string, traced: string): Neighbor[] {
  const candidates: string[] = [];
  for (let current = parentOf(step); current !== undefined && raw(current).kind === 'step'; current = parentOf(current)) {
    const op = targetOf(current);
    if (op !== undefined && raw(op).kind === 'op') {
      candidates.push(...realizersOf(op));
      break;
    }
  }
  const sender = senderOf(step);
  for (const code of sender === undefined ? [] : realizersOf(sender)) {
    candidates.push(...childrenOf(code).filter(child => raw(child).kind === 'op'));
  }
  return unique(candidates).flatMap(caller => {
    const call = findEdge('calls', caller, traced);
    return call ? [neighbor(traced, call, caller)] : [];
  });
}

/** The cases whose covers edges reach a node, or the functions realizing it. */
function coveringCases(id: string): string[] {
  return unique([id, ...realizersOf(id)].flatMap(target => incoming(target, 'covers').map(each => each.from)));
}

/** A node's page; its tabs are added where the answer is made (handlers.ts), as they draw on the views. */
export function entityOf(id: string, base?: Revision): Omit<Entity, 'tabs'> {
  const node = raw(id);
  const listed: string[] = [];
  const list = (ids: string[]) => {
    listed.push(...ids);
    return ids;
  };
  const realizedBy = (node.kind === 'actor' ? [id, ...descendantsOf(id)] : [id]).flatMap(target => incoming(target, 'realizes').map(each => neighbor(target, each, each.from)));
  const properties = list(relatedProperties(id)).map(plain);
  const taking = list(node.layer === 'design' && node.kind !== 'step' ? stepsTakingPart(id) : []);
  const flows = flowSteps(taking);
  list(flows.map(each => each.flow.id));
  const sourced = node.layer === 'code' ? [id] : realizedBy.map(each => each.node.id);
  const covered = list(coveringCases(id));
  const evidence = evidenceOf(id);
  const entity: Omit<Entity, 'tabs'> = {
    node: full(id),
    ancestors: ancestorsOf(id),
    diagnostics: [],
    views: relatedViews(id),
    realizedBy,
    properties,
    flows,
    sources: sourced.map(declarationOf),
    ...(covered.length > 0 ? { coveredBy: covered.map(plain) } : {}),
    ...(evidence ? { evidence } : {}),
    ...(base ? { changes: [id, ...sourced.filter(each => each !== id)].map(each => nodeChangeOf(each, base)) } : {}),
    ...(node.layer === 'design' && node.kind !== 'step' ? { review: { state: 'pending' as const } } : {}),
  };
  if (node.layer === 'design' && (node.kind === 'container' || node.kind === 'actor')) {
    entity.container = {
      contents: list(childrenOf(id).filter(child => !['step', 'case'].includes(raw(child).kind))).map(plain),
      dependsOn: dependencyEdges(outgoing(id).filter(each => each.kind === 'uses' || each.kind === 'depends'), each => each.to).map(each => neighbor(id, each, each.to)),
      dependedOnBy: dependencyEdges(incoming(id).filter(each => each.kind === 'uses' || each.kind === 'depends'), each => each.from).map(each => neighbor(id, each, each.from)),
      files: outgoing(id, 'owns').map(each => plain(each.to)),
    };
  } else if (node.layer === 'design' && node.kind === 'type') {
    const sources = [id, ...opsOf(id)];
    const referring = incoming(id, 'refers').map(each => each.from).filter(from => raw(from).layer === 'design' && ['type', 'op'].includes(raw(from).kind));
    entity.type = {
      members: membersOf(id),
      ops: list(opsOf(id)).map(op => ({
        op: plain(op),
        realizedBy: incoming(op, 'realizes').map(each => neighbor(op, each, each.from)),
        properties: incoming(op, 'about').map(each => plain(each.from)),
      })),
      refers: list(unique(sources.flatMap(source => outgoing(source, 'refers').map(each => each.to))).filter(target => target !== id)).map(plain),
      referredBy: list(unique(referring).filter(from => !sources.includes(from))).map(plain),
      supertypes: outgoing(id).filter(each => each.kind === 'extends' || each.kind === 'implements').map(each => neighbor(id, each, each.to)),
      subtypes: incoming(id).filter(each => each.kind === 'extends' || each.kind === 'implements').map(each => neighbor(id, each, each.from)),
      emittedBy: steps(list(incoming(id, 'emits').map(each => each.from))),
      subscribers: list(incoming(id, 'subscribes').map(each => each.from)).map(plain),
      raisedBy: list(referring.filter(from => raw(from).kind === 'op' && (raw(from).attrs.signature ?? []).some(signature => (signature.split(' raises ')[1] ?? '').split(', ').includes(id)))).map(plain),
      handledBy: steps(list(incoming(id, 'handles').map(each => each.from))),
    };
  } else if (node.layer === 'design' && node.kind === 'op') {
    const functions = realizersOf(id);
    const subscribes = outgoing(id, 'subscribes')[0];
    entity.op = {
      signatureTypes: list(outgoing(id, 'refers').map(each => each.to)).map(plain),
      callers: functions.flatMap(fn => callersOf(fn).map(each => neighbor(fn, each, each.from))),
      callees: functions.flatMap(fn => calleesOf(fn).map(each => neighbor(fn, each, each.to))),
      ...(subscribes ? { subscribes: plain(subscribes.to) } : {}),
    };
  } else if (node.kind === 'flow' || node.kind === 'property') {
    const predicate = incoming(id, 'checks')[0];
    const checked = {
      ...(predicate ? { predicate: neighbor(id, predicate, predicate.from) } : {}),
      generators: incoming(id, 'generates').map(each => neighbor(id, each, each.from)),
      cases: list(childrenOf(id).filter(child => raw(child).kind === 'case')).map(child => {
        const caseEvidence = evidenceOf(child);
        return { node: plain(child), ...(caseEvidence ? { evidence: caseEvidence } : {}) };
      }),
    };
    if (node.kind === 'flow') {
      entity.flow = { ...checked, steps: steps(list(stepsOf(id))) };
    } else {
      entity.property = { ...checked, about: list(outgoing(id, 'about').map(each => each.to)).map(plain) };
    }
  } else if (node.kind === 'step') {
    const call = node.form === 'on' ? callBefore(id) : undefined;
    entity.step = {
      ...(call ? { call: full(call) } : {}),
      children: steps(list(childrenOf(id).filter(child => raw(child).kind === 'step'))),
      traces: outgoing(id, 'traces').map(each => neighbor(id, each, each.to)),
      callSites: outgoing(id, 'traces').flatMap(each => callSitesOf(id, each.to)),
    };
  } else if (node.kind === 'case') {
    entity.case = { covers: outgoing(id, 'covers').map(each => neighbor(id, each, each.to)) };
  } else if (node.layer === 'code') {
    const owner = node.form === 'file' ? incoming(id, 'owns')[0]?.from : undefined;
    entity.code = {
      ...(owner ? { owner: plain(owner) } : {}),
      contents: list(childrenOf(id)).map(plain),
      callers: node.kind === 'op' ? callersOf(id).map(each => neighbor(id, each, each.from)) : [],
      callees: node.kind === 'op' ? calleesOf(id).map(each => neighbor(id, each, each.to)) : [],
      details: node.kind === 'op' ? detailCalleesOf(id).map(plain) : [],
      ...(node.kind === 'op' || node.kind === 'type' ? { declaration: declarationLine(id) } : {}),
    };
  }
  entity.diagnostics = diagnosticsAbout(subjectsOf([id, ...listed], [...outgoing(id), ...incoming(id)]));
  return entity;
}
