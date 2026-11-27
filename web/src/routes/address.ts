import { c4LevelValues, classLayerValues, edgeKindValues, viewKindValues } from '../api/schema';
import type { C4Level, ClassLayer, EdgeKind, ViewKind, ViewRequest } from '../api/types';

/*
 * The page address holds every piece of the viewer's state (docs/design.md §7.5): the view with its input, expansions,
 * layer or level and selection; the entity page with its navigation trail; and the base revision a comparison names.
 * The server keeps none of it. Pages read the address through these functions and build every link with them; an
 * address these functions cannot read is an error, never read as something else.
 */

export const ROOT_PATH = '/';
export const VIEW_PATH = '/view/:kind';
export const ENTITY_PATH = '/entity';

/** The view the root address opens. */
export const DEFAULT_VIEW: ViewKind = 'architecture';

/** A view as the reader left it: its input, expansions, layer or level and selection. `select` is a node's identity or an edge's `kind from to`. */
export interface ViewPlace {
  kind: ViewKind;
  input?: string;
  expand: string[];
  select?: string;
  layer?: ClassLayer;
  level?: C4Level;
}

/** A view as the address writes it: the place, and the base revision a comparison names. */
export interface ViewAddress extends ViewPlace {
  base?: string;
}

/** An entry of the navigation trail: the view the reader started from, as they left it, or a node they went through. */
export type TrailEntry = { view: ViewPlace } | { node: string };

export interface EntityAddress {
  id: string;
  trail: TrailEntry[];
  base?: string;
}

/** A view entry of the trail is written `@kind` and the view's query, as `@class?input=check&layer=code`; identities never start with `@`. */
const VIEW_MARK = '@';

/** A value of the address from a closed set: an absent one is undefined, and one outside the set is an error. */
function readChoice<T extends string>(values: readonly T[], text: string | null, what: string): T | undefined {
  if (text === null) {
    return undefined;
  }
  const value = values.find(candidate => candidate === text);
  if (value === undefined) {
    throw new Error(`The address names ${what} ${text}, which the viewer does not have: it has ${values.join(', ')}`);
  }
  return value;
}

/** A view's kind from the address. */
export function readViewKind(param: string | undefined): ViewKind {
  const kind = readChoice(viewKindValues, param ?? null, 'the view');
  if (kind === undefined) {
    throw new Error('The address names no view');
  }
  return kind;
}

/** The query entries of a view's place, as both a view's address and a trail's view entry write them. */
function placeEntries(place: ViewPlace): [string, string | undefined][] {
  return [
    ['input', place.input],
    ...place.expand.map((id): [string, string] => ['expand', id]),
    ['layer', place.layer],
    ['level', place.level],
    ['select', place.select],
  ];
}

function readPlace(kind: ViewKind, params: URLSearchParams): ViewPlace {
  return {
    kind,
    input: params.get('input') ?? undefined,
    expand: params.getAll('expand'),
    select: params.get('select') ?? undefined,
    layer: readChoice(classLayerValues, params.get('layer'), 'the class view layer'),
    level: readChoice(c4LevelValues, params.get('level'), 'the C4 level'),
  };
}

function writeTrail(entry: TrailEntry): string {
  return 'node' in entry ? entry.node : `${VIEW_MARK}${entry.view.kind}${query(placeEntries(entry.view))}`;
}

function readTrail(text: string): TrailEntry {
  if (!text.startsWith(VIEW_MARK)) {
    return { node: text };
  }
  const at = text.indexOf('?');
  const kind = readViewKind(text.slice(VIEW_MARK.length, at < 0 ? undefined : at));
  return { view: readPlace(kind, new URLSearchParams(at < 0 ? '' : text.slice(at + 1))) };
}

function query(entries: [string, string | undefined][]): string {
  const params = new URLSearchParams();
  for (const [key, value] of entries) {
    if (value !== undefined) {
      params.append(key, value);
    }
  }
  const text = params.toString();
  return text === '' ? '' : `?${text}`;
}

export function viewPath(address: ViewAddress): string {
  return `/view/${address.kind}${query([...placeEntries(address), ['base', address.base]])}`;
}

export function entityPath(address: EntityAddress): string {
  return `${ENTITY_PATH}${query([
    ['id', address.id],
    ...address.trail.map((entry): [string, string] => ['trail', writeTrail(entry)]),
    ['base', address.base],
  ])}`;
}

export function readViewAddress(kind: ViewKind, params: URLSearchParams): ViewAddress {
  return { ...readPlace(kind, params), base: params.get('base') ?? undefined };
}

/** The entity page's address, or undefined when it names no node. */
export function readEntityAddress(params: URLSearchParams): EntityAddress | undefined {
  const id = params.get('id');
  if (id === null) {
    return undefined;
  }
  return {
    id,
    trail: params.getAll('trail').map(readTrail),
    base: params.get('base') ?? undefined,
  };
}

/** The views that take the whole system when the address names no input (docs/design.md §7.5.1). */
const SYSTEM_VIEWS: ReadonlySet<ViewKind> = new Set(['architecture', 'events', 'ddd', 'c4']);

/**
 * The address of a view a page offers, such as an entry of the related views, keeping the base being compared with. A
 * view that takes the whole system by default leaves the system out of its address; the others keep it as their input.
 */
export function requestedView(request: ViewRequest, base: string | undefined): ViewAddress {
  return {
    kind: request.kind,
    input: request.input.form === 'system' && SYSTEM_VIEWS.has(request.kind) ? undefined : request.input.id,
    expand: request.expand,
    layer: request.layer,
    level: request.level,
    select: request.select,
    base,
  };
}

/** Whether a selection names an edge, written `kind from to`, rather than a node. */
export function isEdgeKey(select: string): boolean {
  return select.includes(' ');
}

/** An edge a selection names, written `kind from to`. */
export function readEdgeKey(select: string): { kind: EdgeKind; from: string; to: string } {
  const [kindText, from, to, ...rest] = select.split(' ');
  const kind = readChoice(edgeKindValues, kindText ?? null, 'the edge kind');
  if (kind === undefined || from === undefined || to === undefined || rest.length > 0) {
    throw new Error(`The address selects ${select}, which is not an edge written as kind, from and to`);
  }
  return { kind, from, to };
}

/** The same view selecting another node or edge; nothing else changes. */
export function selecting(address: ViewAddress, select: string | undefined): ViewAddress {
  return { ...address, select };
}

/** The same view with an identity expanded or collapsed. */
export function toggling(address: ViewAddress, id: string): ViewAddress {
  const expand = address.expand.includes(id) ? address.expand.filter(other => other !== id) : [...address.expand, id];
  return { ...address, expand };
}

/** Entering a node's entity page from a view: the trail starts at the view, as the reader left it. */
export function enteringFromView(view: ViewAddress, id: string): EntityAddress {
  const { base, ...place } = view;
  return { id, trail: [{ view: place }, { node: id }], base };
}

/** Following a reference from an entity page: the trail grows by the node, or goes back to it if it is on the trail. */
export function following(entity: EntityAddress, id: string): EntityAddress {
  const at = entity.trail.findIndex(entry => 'node' in entry && entry.node === id);
  const trail = at >= 0 ? entity.trail.slice(0, at + 1) : [...entity.trail, { node: id }];
  return { id, trail, base: entity.base };
}

/** The view a trail's view entry names, as the reader left it, compared with the entity page's base. */
export function trailView(entry: { view: ViewPlace }, base: string | undefined): ViewAddress {
  return { ...entry.view, base };
}
