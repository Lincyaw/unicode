import { viewKindValues } from '../../api/schema';
import type { Entity, Node, ViewKind, ViewRequest } from '../../api/types';

/** The views that take the whole system when no input is given (docs/design.md §7.5.1). */
const SYSTEM_VIEWS: ViewKind[] = ['architecture', 'events', 'ddd', 'c4'];

/** The labelled views, available only when the system file declares their notation. */
const NOTATION_VIEWS: Partial<Record<ViewKind, string>> = { ddd: 'ddd', c4: 'c4' };

/**
 * Where each view tab leads: the view of the selection when it offers one, else of the current input, else the
 * whole system for the views that take it; a tab with none stays unavailable.
 */
export function tabTargets(system: Node, sources: (Entity | undefined)[]): Partial<Record<ViewKind, ViewRequest>> {
  const notations = new Set(system.attrs.notations ?? []);
  const targets: Partial<Record<ViewKind, ViewRequest>> = {};
  for (const kind of viewKindValues) {
    const notation = NOTATION_VIEWS[kind];
    if (notation !== undefined && !notations.has(notation)) {
      continue;
    }
    const offered = sources.flatMap(source => source?.views.filter(view => view.kind === kind) ?? [])[0];
    if (offered) {
      targets[kind] = offered;
    } else if (SYSTEM_VIEWS.includes(kind)) {
      targets[kind] = { kind, input: system, expand: [] };
    }
  }
  return targets;
}
