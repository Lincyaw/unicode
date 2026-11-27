import { c4LevelValues, classLayerValues, viewKindValues } from '../api/schema';
import type { C4Level, ClassLayer, ViewKind, ViewRequest } from '../api/types';

/*
 * The page address holds every piece of the viewer's state (docs/design.md §7.5): the view with its input, expansions,
 * layer or level and selection; the entity page with its navigation trail; and the base revision a comparison names.
 * The server keeps none of it. Pages read the address through these functions and build every link with them.
 */

export const ROOT_PATH = '/';
export const VIEW_PATH = '/view/:kind';
export const ENTITY_PATH = '/entity';

/** The view the root address opens. */
export const DEFAULT_VIEW: ViewKind = 'architecture';

/** A view as the address writes it. `select` is a node's identity or an edge's `kind from to`. */
export interface ViewAddress {
  kind: ViewKind;
  input?: string;
  expand: string[];
  select?: string;
  layer?: ClassLayer;
  level?: C4Level;
  base?: string;
}

/** An entry of the navigation trail: the view the reader started from, or a node they went through. */
export type TrailEntry = { view: ViewKind; input?: string } | { node: string };

export interface EntityAddress {
  id: string;
  trail: TrailEntry[];
  base?: string;
}

/** A view entry of the trail is written `@kind` or `@kind input`; identities never start with `@`. */
const VIEW_MARK = '@';

function writeTrail(entry: TrailEntry): string {
  if ('node' in entry) {
    return entry.node;
  }
  return entry.input === undefined ? `${VIEW_MARK}${entry.view}` : `${VIEW_MARK}${entry.view} ${entry.input}`;
}

function readTrail(text: string): TrailEntry | undefined {
  if (!text.startsWith(VIEW_MARK)) {
    return { node: text };
  }
  const [kind, ...rest] = text.slice(VIEW_MARK.length).split(' ');
  const view = viewKindValues.find(value => value === kind);
  return view && { view, input: rest.length > 0 ? rest.join(' ') : undefined };
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
  return `/view/${address.kind}${query([
    ['input', address.input],
    ...address.expand.map((id): [string, string] => ['expand', id]),
    ['layer', address.layer],
    ['level', address.level],
    ['select', address.select],
    ['base', address.base],
  ])}`;
}

export function entityPath(address: EntityAddress): string {
  return `${ENTITY_PATH}${query([
    ['id', address.id],
    ...address.trail.map((entry): [string, string] => ['trail', writeTrail(entry)]),
    ['base', address.base],
  ])}`;
}

export function readViewKind(param: string | undefined): ViewKind | undefined {
  return viewKindValues.find(value => value === param);
}

export function readViewAddress(kind: ViewKind, params: URLSearchParams): ViewAddress {
  return {
    kind,
    input: params.get('input') ?? undefined,
    expand: params.getAll('expand'),
    select: params.get('select') ?? undefined,
    layer: classLayerValues.find(value => value === params.get('layer')),
    level: c4LevelValues.find(value => value === params.get('level')),
    base: params.get('base') ?? undefined,
  };
}

/** The entity page's address, or undefined when it names no node. */
export function readEntityAddress(params: URLSearchParams): EntityAddress | undefined {
  const id = params.get('id');
  if (id === null) {
    return undefined;
  }
  return {
    id,
    trail: params.getAll('trail').flatMap(text => readTrail(text) ?? []),
    base: params.get('base') ?? undefined,
  };
}

/** The address of a view a page offers, such as an entry of the related views, keeping the base being compared with. */
export function requestedView(request: ViewRequest, base: string | undefined): ViewAddress {
  return {
    kind: request.kind,
    input: request.input.form === 'system' ? undefined : request.input.id,
    expand: request.expand,
    layer: request.layer,
    level: request.level,
    select: request.select,
    base,
  };
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

/** Entering a node's entity page from a view: the trail starts at the view. */
export function enteringFromView(view: ViewAddress, id: string): EntityAddress {
  return { id, trail: [{ view: view.kind, input: view.input }, { node: id }], base: view.base };
}

/** Following a reference from an entity page: the trail grows by the node, or goes back to it if it is on the trail. */
export function following(entity: EntityAddress, id: string): EntityAddress {
  const at = entity.trail.findIndex(entry => 'node' in entry && entry.node === id);
  const trail = at >= 0 ? entity.trail.slice(0, at + 1) : [...entity.trail, { node: id }];
  return { id, trail, base: entity.base };
}

/** The view a trail's view entry names. */
export function trailView(entry: { view: ViewKind; input?: string }, base: string | undefined): ViewAddress {
  return { kind: entry.view, input: entry.input, expand: [], base };
}
