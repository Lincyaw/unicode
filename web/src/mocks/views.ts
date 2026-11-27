import { viewKindValues } from '../api/schema';
import { has, SYSTEM_ID } from './graph';
import type { C4Level, ClassLayer, View, ViewKind } from './schemaTypes';
import { assembleView, NotApplicable, NotFound, type Drawing } from './viewKit';
import { calls, code } from './viewsCode';
import { events, flowDrawing, property } from './viewsFlow';
import { architecture, c4, classView, ddd } from './viewsStructure';

/** The views that take the whole system when the request names no input. */
const SYSTEM_DEFAULT: ViewKind[] = ['architecture', 'events', 'ddd', 'c4'];

export interface ViewQuery {
  input?: string;
  expand: string[];
  layer?: ClassLayer;
  level?: C4Level;
}

/** The data of one view (docs/design.md §7.1–§7.2): what it draws, folded and ordered. */
export function viewOf(kind: ViewKind, query: ViewQuery): View {
  if (!viewKindValues.includes(kind)) {
    throw new NotApplicable(`there is no ${String(kind)} view; the views are ${viewKindValues.join(', ')}`);
  }
  const input = query.input ?? (SYSTEM_DEFAULT.includes(kind) ? SYSTEM_ID : undefined);
  if (input === undefined) {
    throw new NotApplicable(`the ${kind} view needs an input`);
  }
  if (!has(input)) {
    throw new NotFound(`the snapshot has no node ${input}`);
  }
  const drawings: Record<ViewKind, () => Drawing> = {
    architecture: () => architecture(input, query.expand),
    ddd: () => ddd(input, query.expand),
    class: () => classView(input, query.expand, query.layer ?? 'design'),
    sequence: () => flowDrawing(input),
    activity: () => flowDrawing(input),
    events: () => events(input),
    calls: () => calls(input, query.expand),
    property: () => property(input),
    code: () => code(input, query.expand),
    c4: () => c4(input, query.level ?? 'container'),
  };
  return assembleView(kind, input, query.expand, drawings[kind]());
}
