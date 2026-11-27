import { viewKindValues } from '../api/schema';
import { relatedViews } from './entities';
import { ancestorsOf, childrenOf, NODES, outgoing, plain, raw, realizersOf, SYSTEM_ID } from './graph';
import { tree } from './queries';
import type { TreeNode, ViewKind, ViewRequest, ViewTab } from './schemaTypes';
import { NotApplicable } from './viewKit';
import { viewOf } from './views';

/*
 * Where each view's tab leads from a node, as unicode serve answers it (docs/design.md §7.5.1): the node's first
 * related view of the kind; else the default input of the nearest scope (the node, its ancestors, the system), in the
 * design tree's order; else the first input the view takes; else why there is none.
 */

/** Whether the view of a kind applies to an input, as /views/{kind} would answer it. */
function applies(kind: ViewKind, input: string): boolean {
  try {
    viewOf(kind, { input, expand: [] });
    return true;
  } catch (error) {
    if (error instanceof NotApplicable) {
      return false;
    }
    throw error;
  }
}

/** The subtree of the design tree at a node. */
function subtree(root: TreeNode, id: string): TreeNode | undefined {
  return root.node.id === id ? root : root.children.map(child => subtree(child, id)).find(found => found !== undefined);
}

/** The first node of a subtree, in the tree's order, that passes the test. */
function firstIn(root: TreeNode, test: (id: string) => boolean): string | undefined {
  return test(root.node.id) ? root.node.id : root.children.map(child => firstIn(child, test)).find(found => found !== undefined);
}

/** The places a default input is looked for, nearest first: the node, then its containers; a code element stands at the design nodes it realizes. */
function scopes(id: string, root: TreeNode): TreeNode[] {
  const node = raw(id);
  const starts = node.layer === 'code' ? outgoing(id).filter(each => ['realizes', 'checks', 'generates', 'observes', 'substitutes'].includes(each.kind)).map(each => each.to) : [id];
  const ids = starts.flatMap(start => [start, ...ancestorsOf(start).map(ancestor => ancestor.id).reverse()]).filter(each => each !== SYSTEM_ID);
  return [...new Set(ids)].flatMap(each => subtree(root, each) ?? []).concat(root);
}

/** The code files a design container owns, by path. */
function ownedFiles(id: string): string[] {
  return outgoing(id, 'owns').map(each => each.to).sort();
}

/** The file a node's code lies in: a file, a code element's file, the file of the first code realizing a design node, a container's first owned file. */
function codeFile(id: string): string | undefined {
  const node = raw(id);
  if (node.layer === 'code') {
    if (node.form === 'file') {
      return id;
    }
    return node.kind === 'container' ? childrenOf(id).filter(child => raw(child).form === 'file').sort()[0] : ancestorsOf(id).findLast(ancestor => ancestor.form === 'file')?.id;
  }
  const realizer = realizersOf(id)[0];
  return realizer !== undefined ? codeFile(realizer) : ownedFiles(id)[0];
}

/** The default input of a view whose kind the node does not offer. */
function defaultInput(kind: ViewKind, id: string, places: TreeNode[], root: TreeNode): string | undefined {
  const fits = (test: (each: string) => boolean) => places.map(place => firstIn(place, each => test(each) && applies(kind, each))).find(found => found !== undefined);
  switch (kind) {
    case 'architecture':
    case 'events':
    case 'ddd':
    case 'c4':
      return applies(kind, SYSTEM_ID) ? SYSTEM_ID : undefined;
    case 'class':
      return places.map(place => place.node.id).find(each => raw(each).kind === 'container' && applies(kind, each));
    case 'sequence':
    case 'activity':
      return fits(each => raw(each).kind === 'flow');
    case 'calls':
      return fits(each => raw(each).kind === 'op' && raw(each).salient);
    case 'property':
      return places.find(place => place.node.kind === 'container' && firstIn(place, each => raw(each).kind === 'property') !== undefined && applies(kind, place.node.id))?.node.id;
    case 'code': {
      const file = codeFile(id);
      if (file !== undefined && applies(kind, file)) {
        return file;
      }
      const owned = (each: string) => ownedFiles(each).find(candidate => applies(kind, candidate));
      const holder = places.map(place => place.node.id).find(each => owned(each) !== undefined) ?? firstIn(root, each => owned(each) !== undefined);
      return holder === undefined ? undefined : owned(holder);
    }
  }
}

/** Where each view's tab leads from a node, in the tabs' order. */
export function tabsOf(id: string): ViewTab[] {
  const root = tree();
  const related = relatedViews(id);
  const places = scopes(id, root);
  const notations = raw(SYSTEM_ID).attrs.notations ?? [];
  return viewKindValues.map((kind): ViewTab => {
    const offered = related.find(each => each.kind === kind);
    if (offered) {
      return { kind, view: offered };
    }
    // Else the first input the view takes, by identity.
    const input = defaultInput(kind, id, places, root) ?? NODES.map(node => node.id).sort().find(each => applies(kind, each));
    if (input !== undefined) {
      const view: ViewRequest = { kind, input: plain(input), expand: [] };
      return { kind, view };
    }
    return { kind, closed: (kind === 'ddd' || kind === 'c4') && !notations.includes(kind) ? 'undeclared-notation' : 'no-input' };
  });
}
