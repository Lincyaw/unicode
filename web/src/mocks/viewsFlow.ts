import { childrenOf, incoming, outgoing, parentOf, raw, receiverOf, senderOf, stepsOf, SYSTEM_ID, targetOf } from './graph';
import type { RawEdge, ViewEdge, ViewNode } from './schemaTypes';
import { NotApplicable, viewEdge, viewNode, type Drawing } from './viewKit';

/* The views a designer writes in full and that never fold: sequence, activity, events and property (docs/design.md §7.2). */

/** The edges of a flow's steps that the sequence and activity views draw. */
const STEP_EDGES = ['contains', 'from', 'calls', 'emits', 'handles'];

/**
 * A flow's lifelines in the order they first appear, each step's sender before its receiver, then its steps in their
 * order; the edges that place the steps: contains, from, calls, emits and handles.
 */
export function flowDrawing(input: string): Drawing {
  if (raw(input).kind !== 'flow') {
    throw new NotApplicable(`the view takes a flow, and ${input} is a ${raw(input).kind}`);
  }
  const steps = stepsOf(input);
  const lifelines: string[] = [];
  for (const step of steps) {
    for (const participant of [senderOf(step), receiverOf(step)]) {
      if (participant !== undefined && !lifelines.includes(participant)) {
        lifelines.push(participant);
      }
    }
  }
  const nodes: ViewNode[] = [...lifelines.map(id => viewNode(id)), ...steps.map(id => viewNode(id))];
  const edges = [input, ...steps].flatMap(id => outgoing(id).filter(each => STEP_EDGES.includes(each.kind) && (each.kind !== 'contains' || raw(each.to).kind === 'step')));
  return { nodes, edges: edges.map(each => viewEdge(each)) };
}

/** Every design node under a container, the container excluded. */
function descendantsOf(container: string): string[] {
  return childrenOf(container).flatMap(child => [child, ...descendantsOf(child)]);
}

/** The call step an on step follows: the last call before it among its siblings. */
export function callBefore(onStep: string): string | undefined {
  const parent = parentOf(onStep);
  if (parent === undefined) {
    return undefined;
  }
  const siblings = childrenOf(parent);
  return siblings.slice(0, siblings.indexOf(onStep)).filter(sibling => raw(sibling).form === 'call').at(-1);
}

/**
 * Events with the steps that emit them and the operations that subscribe to them (with the async steps that run
 * those operations); errors handled in a flow with the call step that raises them and the on steps that handle them
 * (with what those do). A raising step reaches its error through two edges of the graph: the step calls an
 * operation, whose signature refers to the error.
 */
export function events(input: string): Drawing {
  if (raw(input).kind !== 'container' || raw(input).layer !== 'design') {
    throw new NotApplicable(`the events view takes a design container, and ${input} is a ${raw(input).kind}`);
  }
  const scope = new Set(input === SYSTEM_ID ? descendantsOf(SYSTEM_ID) : descendantsOf(input));
  const drawn: string[] = [];
  const edges: RawEdge[] = [];
  const draw = (id: string) => {
    if (!drawn.includes(id)) {
      drawn.push(id);
    }
  };
  for (const event of [...scope].filter(id => raw(id).kind === 'type' && raw(id).form === 'event')) {
    draw(event);
    for (const emits of incoming(event, 'emits')) {
      draw(emits.from);
      edges.push(emits);
    }
    for (const subscribes of incoming(event, 'subscribes')) {
      draw(subscribes.from);
      edges.push(subscribes);
      for (const runs of incoming(subscribes.from, 'calls').filter(each => raw(each.from).kind === 'step')) {
        draw(runs.from);
        edges.push(runs);
      }
    }
  }
  for (const error of [...scope].filter(id => raw(id).kind === 'type' && raw(id).form === 'error')) {
    const handlers = incoming(error, 'handles');
    if (handlers.length === 0) {
      continue;
    }
    draw(error);
    for (const handles of handlers) {
      const call = callBefore(handles.from);
      const op = call === undefined ? undefined : targetOf(call);
      if (call !== undefined && op !== undefined) {
        draw(call);
        edges.push(...outgoing(call, 'calls'), ...outgoing(op, 'refers').filter(each => each.to === error));
      }
      draw(handles.from);
      edges.push(handles);
      for (const child of childrenOf(handles.from).filter(id => raw(id).kind === 'step')) {
        draw(child);
        edges.push(...outgoing(handles.from, 'contains').filter(each => each.to === child));
      }
    }
  }
  return { nodes: drawn.map(id => viewNode(id)), edges: edges.map(each => viewEdge(each)) };
}

/**
 * Properties with what they are about (an operation with its signature), their cases with their results, the
 * predicates that check them, and the code the cases cover.
 */
export function property(input: string): Drawing {
  const node = raw(input);
  let properties: string[];
  if (node.kind === 'property') {
    properties = [input];
  } else if (node.kind === 'container' && node.layer === 'design') {
    properties = descendantsOf(input).filter(id => raw(id).kind === 'property');
  } else {
    throw new NotApplicable(`the property view takes a property or a design container, and ${input} is a ${node.kind}`);
  }
  const drawn: string[] = [];
  const edges: ViewEdge[] = [];
  const draw = (id: string) => {
    if (!drawn.includes(id)) {
      drawn.push(id);
    }
  };
  for (const each of properties) {
    draw(each);
    for (const about of outgoing(each, 'about')) {
      draw(about.to);
      edges.push(viewEdge(about));
    }
    for (const contains of outgoing(each, 'contains')) {
      draw(contains.to);
      edges.push(viewEdge(contains));
      for (const covers of outgoing(contains.to, 'covers')) {
        draw(covers.to);
        edges.push(viewEdge(covers));
      }
    }
    for (const checks of incoming(each, 'checks')) {
      draw(checks.from);
      edges.push(viewEdge(checks));
    }
  }
  return { nodes: drawn.map(id => viewNode(id)), edges };
}
