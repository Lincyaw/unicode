import { createContext, useContext, type KeyboardEvent } from 'react';
import type { ViewRequest } from '../../api/types';

/**
 * Where the marks of a page lead. The page builds it from the address and hands it down, so components show nodes and
 * links without reading or changing the address themselves: one click on a node previews it, a double click or ↗
 * enters its entity page (docs/design.md §7.5).
 */
export interface NodeLinks {
  /** The selected node's identity or edge's `kind from to` */
  selected?: string;

  /** Previews a node or an edge in the right pane */
  select: (id: string) => void;

  /** Enters a node's entity page */
  open: (id: string) => void;

  /** The address of a node's entity page */
  entityHref: (id: string) => string;

  /** The address of a view a page offers */
  viewHref: (request: ViewRequest) => string;

  /** On a view: the same view with an identity expanded or collapsed */
  expandHref?: (id: string) => string;
}

export const LinksContext = createContext<NodeLinks | null>(null);

export function useLinks(): NodeLinks {
  const links = useContext(LinksContext);
  if (links === null) {
    throw new Error('A page provides LinksContext around the marks it renders');
  }
  return links;
}

/**
 * What makes an element preview a node on a click and enter its page on a double click or Enter. The element carries
 * the node's identity, by which a canvas finds the selected node to bring it into view.
 */
export function useNodeActions(id: string) {
  const links = useLinks();
  return {
    'data-node': id,
    'aria-pressed': links.selected === id,
    onClick: () => links.select(id),
    onDoubleClick: () => links.open(id),
    onKeyDown: (event: KeyboardEvent) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        links.open(id);
      }
    },
  };
}

/**
 * Marks a box that stands for a node on a drawing, such as a card, as the selected one: the kit's still ∞ edge
 * (`loop-edge-still`, which the box adds to its class while selected) goes round it, and the canvas finds it there.
 */
export function useSelectedBox(id: string) {
  const selected = useLinks().selected === id;
  return { selected, props: { 'data-selected': selected, 'aria-current': selected ? ('true' as const) : undefined } };
}
