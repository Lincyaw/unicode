import { boundTo, childrenOf, designContainerOf, findEdge, incoming, outgoing, parentOf, raw, realizersOf, senderOf, SYSTEM_ID, targetOf } from './graph';
import type { C4Level, ClassLayer, RawEdge, ViewEdge, ViewNode } from './schemaTypes';
import { Aggregates, DDD_TYPE_LABELS, focusOf, NotApplicable, opsOf, typeCard, typeName, viewEdge, viewNode, type Drawing } from './viewKit';

/* The views of the design's structure: architecture, DDD, class and C4 (docs/design.md §7.1–§7.2). */

function requireContainer(input: string) {
  if (raw(input).kind !== 'container' || raw(input).layer !== 'design') {
    throw new NotApplicable(`the view takes a design container, and ${input} is a ${raw(input).kind}`);
  }
}

function requireNotation(notation: string) {
  if (!(raw(SYSTEM_ID).attrs.notations ?? []).includes(notation)) {
    throw new NotApplicable(`the system file does not declare notation ${notation}`);
  }
}

/** The containers and external systems a structural view of `input` draws: the system's children, or the input with its siblings. */
function membersAround(input: string): string[] {
  const scope = input === SYSTEM_ID ? SYSTEM_ID : (parentOf(input) ?? SYSTEM_ID);
  const containers = childrenOf(scope).filter(child => raw(child).kind === 'container');
  return [...containers, ...childrenOf(scope).filter(child => raw(child).kind === 'actor' && raw(child).form === 'external')];
}

/** The depends edges between drawn nodes, and the uses edges no depends stands beside. */
function dependencies(drawn: Set<string>): ViewEdge[] {
  const between = (each: RawEdge) => drawn.has(each.from) && drawn.has(each.to);
  const depends = [...drawn].flatMap(id => outgoing(id, 'depends')).filter(between);
  const uses = [...drawn].flatMap(id => outgoing(id, 'uses')).filter(between).filter(each => !findEdge('depends', each.from, each.to));
  return [...depends, ...uses].map(each => viewEdge(each));
}

export function architecture(input: string, expand: string[]): Drawing {
  requireContainer(input);
  const expanded = new Set([input, ...expand]);
  const nodes: ViewNode[] = [];
  const folded: string[] = [];
  let hidden = 0;
  for (const member of membersAround(input)) {
    if (raw(member).kind === 'actor') {
      nodes.push(viewNode(member, { contents: childrenOf(member).map(child => viewNode(child)) }));
      continue;
    }
    const focus = focusOf(member);
    if (focus === 'utility' && !expanded.has(member)) {
      hidden += 1;
    } else if (focus === 'core' || expanded.has(member)) {
      const contents = childrenOf(member).filter(child => ['type', 'flow', 'property'].includes(raw(child).kind));
      nodes.push(viewNode(member, { contents: contents.map(child => (raw(child).kind === 'type' ? typeCard(child) : viewNode(child))) }));
    } else {
      folded.push(member);
      nodes.push(viewNode(member, { contents: childrenOf(member).filter(child => raw(child).kind === 'type').map(typeName) }));
    }
  }
  return { nodes, edges: dependencies(new Set(nodes.map(node => node.id))), folded, hidden };
}

function isDomainType(type: string): boolean {
  const node = raw(type);
  return node.kind === 'type' && (node.form === 'event' || (node.attrs.labels ?? []).some(label => DDD_TYPE_LABELS.includes(label)));
}

/** The card a type or an operation is drawn on: a type's own, an operation's type's. */
function cardOf(id: string): string {
  return raw(id).kind === 'op' ? (parentOf(id) ?? id) : id;
}

export function ddd(input: string, expand: string[]): Drawing {
  requireNotation('ddd');
  requireContainer(input);
  const expanded = new Set([input, ...expand]);
  const nodes: ViewNode[] = [];
  const folded: string[] = [];
  const edges: ViewEdge[] = [];
  let hidden = 0;
  for (const member of membersAround(input)) {
    if (raw(member).kind !== 'container' || !(raw(member).attrs.labels ?? []).includes('bounded-context')) {
      hidden += raw(member).kind === 'container' && focusOf(member) === 'utility' ? 1 : 0;
      continue;
    }
    const types = childrenOf(member).filter(isDomainType);
    const focus = focusOf(member);
    if (focus === 'utility' && !expanded.has(member)) {
      hidden += 1;
    } else if (focus === 'core' || expanded.has(member)) {
      nodes.push(viewNode(member, { contents: types.map(typeName) }));
      // refers, extends and implements between this context's own domain types, from a type or one of its operations
      const own = new Set(types);
      for (const source of types.flatMap(type => [type, ...opsOf(type)])) {
        for (const each of outgoing(source).filter(edge => ['refers', 'extends', 'implements'].includes(edge.kind))) {
          if (own.has(each.to) && cardOf(source) !== each.to) {
            edges.push(viewEdge(each));
          }
        }
      }
    } else {
      folded.push(member);
      nodes.push(viewNode(member, { contents: types.map(typeName) }));
    }
  }
  return { nodes, edges: [...dependencies(new Set(nodes.map(node => node.id))), ...edges], folded, hidden };
}

/** The types a class view draws as cards, and the container that is its home. */
function classScope(input: string): { home: string; cards: string[] } {
  const node = raw(input);
  if (node.layer === 'design' && node.kind === 'container' && node.form === 'component') {
    return { home: input, cards: childrenOf(input).filter(child => raw(child).kind === 'type') };
  }
  if (node.layer === 'design' && node.kind === 'type') {
    const sources = [input, ...opsOf(input)];
    const out = sources.flatMap(source => outgoing(source).filter(each => ['refers', 'extends', 'implements'].includes(each.kind)).map(each => each.to));
    const into = incoming(input).filter(each => ['refers', 'extends', 'implements'].includes(each.kind)).map(each => cardOf(each.from));
    const cards = [...new Set([input, ...out, ...into])].filter(id => raw(id).kind === 'type');
    return { home: designContainerOf(input) ?? SYSTEM_ID, cards };
  }
  throw new NotApplicable(`the class view takes a component or a design type, and ${input} is a ${node.layer} ${node.kind}`);
}

export function classView(input: string, expand: string[], layer: ClassLayer): Drawing {
  const { home, cards } = classScope(input);
  const expanded = new Set(expand);
  // A type outside the home is a card when its container is expanded, else it stands in its folded container.
  const shown = new Set(cards.filter(type => designContainerOf(type) === home || expanded.has(designContainerOf(type) ?? '')));
  const outside = cards.filter(type => !shown.has(type));
  const sources = [...shown].flatMap(type => [type, ...opsOf(type)]);
  const references = sources.flatMap(source => outgoing(source).filter(each => ['refers', 'extends', 'implements'].includes(each.kind)));
  for (const each of references) {
    const container = designContainerOf(each.to) ?? home;
    if (raw(each.to).kind === 'type' && container !== home && !expanded.has(container)) {
      outside.push(each.to);
    } else if (raw(each.to).kind === 'type' && container !== home) {
      shown.add(each.to);
    }
  }
  const foldedContainers = [...new Set(outside.map(type => designContainerOf(type) ?? home))].filter(container => container !== home);
  const nodes: ViewNode[] = [];
  const edges: ViewEdge[] = [];
  const aggregates = new Aggregates();
  if (layer === 'design') {
    nodes.push(...[...shown].map(typeCard));
    for (const each of [...shown].flatMap(type => [type, ...opsOf(type)]).flatMap(source => outgoing(source).filter(edge => ['refers', 'extends', 'implements'].includes(edge.kind)))) {
      if (shown.has(each.to)) {
        edges.push(viewEdge(each));
      } else if (raw(each.to).kind === 'type' && foldedContainers.includes(designContainerOf(each.to) ?? '')) {
        aggregates.add(each, cardOf(each.from), designContainerOf(each.to) ?? home);
      }
    }
  } else {
    // The code layer: the classes that realize the drawn types, with their methods, and the code's own references.
    const realizing = new Map([...shown].flatMap(type => realizersOf(type).map(code => [code, type] as const)));
    for (const code of realizing.keys()) {
      nodes.push(viewNode(code, { contents: childrenOf(code).filter(child => raw(child).kind === 'op' && raw(child).salient).map(child => viewNode(child)) }));
    }
    const methodsOf = (code: string) => childrenOf(code).filter(child => raw(child).kind === 'op');
    for (const each of [...realizing.keys()].flatMap(code => [code, ...methodsOf(code)]).flatMap(source => outgoing(source, 'refers'))) {
      const card = raw(each.from).kind === 'op' ? (parentOf(each.from) ?? each.from) : each.from;
      if (realizing.has(each.to)) {
        edges.push(viewEdge(each));
      } else {
        const design = boundTo(each.to);
        const container = design === undefined ? undefined : designContainerOf(design);
        if (container !== undefined && foldedContainers.includes(container)) {
          aggregates.add(each, card, container);
        }
      }
    }
  }
  for (const container of foldedContainers) {
    nodes.push(viewNode(container, { contents: childrenOf(container).filter(child => raw(child).kind === 'type').map(typeName) }));
  }
  return { nodes, edges: [...edges, ...aggregates.edges()], folded: foldedContainers, layer };
}

/** The operations a depends edge's calls realize, by design name. */
function realizedOps(each: RawEdge): string[] {
  const called = (each.attrs.calls ?? []).map(call => call.split(' ')[1] ?? '');
  return [...new Set(called.map(fn => boundTo(fn)).filter((op): op is string => op !== undefined).map(op => raw(op).title))];
}

/** What a person calls in the flows, written as the steps write it: `submit(cart)`, by the container that answers. */
function personCalls(person: string): Map<string, { step: string; labels: string[] }> {
  const byContainer = new Map<string, { step: string; labels: string[] }>();
  for (const step of incoming(person, 'from').map(each => each.from)) {
    const target = targetOf(step);
    const container = target === undefined ? undefined : designContainerOf(target);
    if (target === undefined || container === undefined || senderOf(step) !== person) {
      continue;
    }
    const label = `${raw(target).title}(${(raw(step).attrs.args ?? []).join(', ')})`;
    const entry = byContainer.get(container) ?? { step, labels: [] };
    entry.labels = [...new Set([...entry.labels, label])];
    byContainer.set(container, entry);
  }
  return byContainer;
}

export function c4(input: string, level: C4Level): Drawing {
  requireNotation('c4');
  requireContainer(input);
  const persons = childrenOf(SYSTEM_ID).filter(child => raw(child).kind === 'actor' && raw(child).form === 'person');
  const externals = childrenOf(SYSTEM_ID).filter(child => raw(child).kind === 'actor' && raw(child).form === 'external');
  const scope = input === SYSTEM_ID ? SYSTEM_ID : input;
  const children = childrenOf(scope).filter(child => raw(child).kind === 'container');
  const containers = children.filter(child => (raw(child).attrs.labels ?? []).includes('container'));
  const hidden = children.filter(child => focusOf(child) === 'utility').length;
  const nodes: ViewNode[] = [...persons.map(id => viewNode(id)), viewNode(scope)];
  const edges: ViewEdge[] = [];
  const aggregates = new Aggregates();
  // Where a container stands at this level: itself, or the system that holds it.
  const standIn = (id: string) => (level === 'context' && children.includes(id) ? scope : id);
  if (level === 'container') {
    nodes.push(...containers.map(id => viewNode(id)));
    edges.push(...containers.map(id => incoming(id, 'contains').find(each => each.from === scope)).filter((each): each is RawEdge => each !== undefined).map(each => viewEdge(each)));
  }
  nodes.push(...externals.map(id => viewNode(id)));
  const drawn = new Set(nodes.map(node => node.id));
  for (const each of [...children, ...externals].flatMap(id => outgoing(id, 'depends'))) {
    const [from, to] = [standIn(each.from), standIn(each.to)];
    if (from === to || !drawn.has(from) || !drawn.has(to)) {
      continue;
    }
    if (from === each.from && to === each.to) {
      edges.push(viewEdge(each, realizedOps(each).length > 0 ? { labels: realizedOps(each) } : {}));
    } else {
      aggregates.add(each, from, to, realizedOps(each));
    }
  }
  // A person's edges carry the operations it calls in the flows; the graph holds them as steps, not as edges.
  for (const person of persons) {
    for (const [container, { step, labels }] of personCalls(person)) {
      const to = standIn(container);
      if (drawn.has(to)) {
        edges.push({ kind: 'calls', from: person, to, source: 'derived', origin: raw(step).origin, attrs: {}, labels });
      }
    }
  }
  return { nodes, edges: [...edges, ...aggregates.edges()], hidden, level };
}
