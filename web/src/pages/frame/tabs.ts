import { useOptionalEntity, useSnapshot, useTree } from '../../api/client';
import { viewKindValues } from '../../api/schema';
import type { Entity, Node, TreeNode, ViewKind } from '../../api/types';
import { fileOf } from '../../components/marks/format';
import type { TabLink } from '../../components/shell/ViewTabs';
import { requestedView, type ViewAddress } from '../../routes/address';
import { TAB_REASONS } from '../../terms';

/** The labelled views, available only when the system file declares their notation. */
const NOTATION_VIEWS: Partial<Record<ViewKind, string>> = { ddd: 'ddd', c4: 'c4' };

/** Where a view tab leads, the base aside; or, when the graph holds no input the view takes, why not. */
export type TabTarget = { address: ViewAddress } | { reason: string };

/** The subtrees of the design tree from the root down to a node, if the tree holds it. */
function pathTo(root: TreeNode, id: string): TreeNode[] {
  if (root.node.id === id) {
    return [root];
  }
  for (const child of root.children) {
    const path = pathTo(child, id);
    if (path.length > 0) {
      return [root, ...path];
    }
  }
  return [];
}

/** The first node of a subtree, in the tree's order, that passes the test. */
function firstIn(root: TreeNode, test: (node: Node) => boolean): Node | undefined {
  if (test(root.node)) {
    return root.node;
  }
  for (const child of root.children) {
    const found = firstIn(child, test);
    if (found) {
      return found;
    }
  }
  return undefined;
}

/**
 * The places a default input is looked for, nearest first: each source's subtree, then the containers above it, up to
 * the system. A code element stands at the design nodes it realizes; a node the tree leaves out, such as a step, at
 * its nearest ancestor in the tree.
 */
function scopes(sources: Entity[], root: TreeNode): TreeNode[] {
  const ids = sources.flatMap(source => [
    source.node.id,
    ...(source.node.bindings ?? []).map(binding => binding.node.id),
    ...source.ancestors.map(node => node.id).reverse(),
  ]);
  const places = new Map(ids.flatMap(id => pathTo(root, id).slice(1).reverse()).map(place => [place.node.id, place]));
  return [...places.values(), root];
}

/** The first node that passes the test in the nearest scope that holds one. */
function nearest(places: TreeNode[], test: (node: Node) => boolean): Node | undefined {
  for (const place of places) {
    const found = firstIn(place, test);
    if (found) {
      return found;
    }
  }
  return undefined;
}

/** The file a source's code lies in: the source itself, the file it is declared in, or that of the code realizing it. */
function codeFile(source: Entity): string | undefined {
  const { node } = source;
  if (node.layer === 'code') {
    return node.form === 'file' ? node.id : node.kind === 'container' ? source.container?.files[0]?.id : fileOf(node.origin.path);
  }
  const realized = source.realizedBy[0]?.node;
  return realized ? fileOf(realized.origin.path) : source.container?.files[0]?.id;
}

const isFlow = (node: Node) => node.kind === 'flow';
const isSalientOp = (node: Node) => node.kind === 'op' && node.salient;
const isProperty = (node: Node) => node.kind === 'property';
const isContainer = (node: Node) => node.kind === 'container' && node.form !== 'system';

/**
 * Where each view tab leads (docs/design.md §7.5): the view the selection or the current input offers; else a default
 * input taken from the nearest scope, the selection's own subtree first, then its containers, then the whole system
 * (the first flow, the first salient operation, the file of its code); a tab stays unavailable, with its reason, only
 * when the graph holds no input the view takes.
 */
export function useTabTargets(sources: (Entity | undefined)[]): Record<ViewKind, TabTarget> {
  const { system } = useSnapshot();
  const root = useTree();
  const known = sources.filter(source => source !== undefined);
  const places = scopes(known, root);
  const firstFile = known.flatMap(source => codeFile(source) ?? [])[0];
  // Without a source that names a file, the files of the nearest component.
  const fileHolder = firstFile === undefined ? (places.map(place => place.node).find(isContainer) ?? firstIn(root, isContainer)) : undefined;
  const holderFiles = useOptionalEntity(fileHolder?.id)?.container?.files ?? [];
  const notations = new Set(system.attrs.notations ?? []);
  const at = (kind: ViewKind, input: Node): TabTarget => ({ address: { kind, input: input.id, expand: [] } });

  const fallback = (kind: ViewKind): TabTarget => {
    switch (kind) {
      case 'architecture':
      case 'events':
      case 'ddd':
      case 'c4':
        return { address: { kind, expand: [] } };
      case 'class':
        return at(kind, places.map(place => place.node).find(node => node.kind === 'container') ?? system);
      case 'sequence':
      case 'activity': {
        const flow = nearest(places, isFlow);
        return flow ? at(kind, flow) : { reason: TAB_REASONS.noFlow };
      }
      case 'calls': {
        const op = nearest(places, isSalientOp);
        return op ? at(kind, op) : { reason: TAB_REASONS.noOp };
      }
      case 'property': {
        const holder = places.find(place => place.node.kind === 'container' && firstIn(place, isProperty) !== undefined);
        return holder ? at(kind, holder.node) : { reason: TAB_REASONS.noProperty };
      }
      case 'code': {
        const file = firstFile ?? holderFiles[0]?.id;
        return file ? { address: { kind, input: file, expand: [] } } : { reason: TAB_REASONS.noFile };
      }
    }
  };

  return Object.fromEntries(viewKindValues.map((kind): [ViewKind, TabTarget] => {
    const notation = NOTATION_VIEWS[kind];
    if (notation !== undefined && !notations.has(notation)) {
      return [kind, { reason: TAB_REASONS.notation(notation) }];
    }
    const offered = known.flatMap(source => source.views.filter(view => view.kind === kind))[0];
    return [kind, offered ? { address: requestedView(offered, undefined) } : fallback(kind)];
  })) as Record<ViewKind, TabTarget>;
}

/** The tabs' links: each target's address, written by the page with the base it compares with. */
export function tabLinks(targets: Record<ViewKind, TabTarget>, href: (kind: ViewKind, address: ViewAddress) => string): Record<ViewKind, TabLink> {
  return Object.fromEntries(viewKindValues.map((kind): [ViewKind, TabLink] => {
    const target = targets[kind];
    return [kind, 'reason' in target ? target : { href: href(kind, target.address) }];
  })) as Record<ViewKind, TabLink>;
}
