import { createContext, useContext } from 'react';
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
