import { CODE_SOURCES } from './codeSources';
import { DESIGN_SOURCES } from './designSources';
import type { EdgeKind, NodeKind, Position, RawEdge, RawNode } from './schemaTypes';

/*
 * The builders the shop's fixture is written with. Every place is found in the files the repository holds, so a
 * node's origin and the lines the viewer shows for it always agree.
 */

/** Every file of the repository the mock knows: the design and corpus files, then the code files. */
export const SOURCES: Record<string, string> = { ...DESIGN_SOURCES, ...CODE_SOURCES };

/** The text of a file the mock knows; a path it does not know is a mistake in the fixture. */
export function sourceOf(path: string): string {
  const text = SOURCES[path];
  if (text === undefined) {
    throw new Error(`the fixture names ${path}, which is not one of its files`);
  }
  return text;
}

/** The place where `needle` occurs in a file for the `occurrence`th time, at the column it starts. */
export function at(path: string, needle: string, occurrence = 1): Position {
  const lines = sourceOf(path).split('\n');
  let seen = 0;
  for (const [index, line] of lines.entries()) {
    const column = line.indexOf(needle);
    if (column >= 0) {
      seen += 1;
      if (seen === occurrence) {
        return { path, line: index + 1, column: column + 1 };
      }
    }
  }
  throw new Error(`${path} does not contain ${needle} ${occurrence} times`);
}

/** The place as a call site, written `path:line:column`. */
export function site(path: string, needle: string, occurrence = 1): string {
  const place = at(path, needle, occurrence);
  return `${place.path}:${place.line}:${place.column}`;
}

/** A design node: declared, salient, named by the last segment of its identity (a step by `#n`). */
export function designNode(kind: NodeKind, form: string, id: string, summary: string, origin: Position, attrs: Record<string, string[]> = {}): RawNode {
  const step = id.lastIndexOf('#');
  const title = step >= 0 ? id.slice(step) : (id.split('.').at(-1) ?? id);
  return { id, kind, form, layer: 'design', source: 'declared', origin, summary, salient: true, attrs, title };
}

/** A code node, extracted by the Python backend; salience follows D6 and is written by the fixture. */
export function codeNode(kind: NodeKind, form: string, id: string, title: string, origin: Position, salient: boolean, attrs: Record<string, string[]> = {}): RawNode {
  return { id, kind, form, layer: 'code', source: 'extracted', origin, summary: '', salient, attrs, title };
}

export function edge(kind: EdgeKind, from: string, to: string, source: RawEdge['source'], origin: Position, attrs: Record<string, string[]> = {}): RawEdge {
  return { kind, from, to, source, origin, attrs };
}
