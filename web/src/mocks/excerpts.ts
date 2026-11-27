import { CODE_SOURCES } from './codeSources';
import { SOURCES } from './fixture';
import { raw } from './graph';
import type { CodeExcerpt } from './schemaTypes';

/* Lines of the repository's files, as /code and the pages' code sections give them. */

/** Whether a path is a file the server may read: a design or corpus file, or a code file of the snapshot. */
export function isServedFile(path: string): boolean {
  return SOURCES[path] !== undefined;
}

/** Lines `from` to `to` of a file, both counted from 1 and included; the whole file without a range. */
export function excerptOf(path: string, from?: number, to?: number): CodeExcerpt {
  const lines = (SOURCES[path] ?? '').split('\n');
  if (lines.at(-1) === '') {
    lines.pop();
  }
  const first = Math.max(1, from ?? 1);
  const last = Math.min(lines.length, to ?? lines.length);
  return {
    path,
    from: first,
    to: last,
    text: lines.slice(first - 1, last).map(line => `${line}\n`).join(''),
    ...(CODE_SOURCES[path] !== undefined ? { language: 'python' } : {}),
  };
}

function indentOf(line: string): number {
  return line.length - line.trimStart().length;
}

/**
 * The source of a code element's declaration: its decorators, its first line and the indented block under it; a
 * file's whole text.
 */
export function declarationOf(id: string): CodeExcerpt {
  const { origin, form } = raw(id);
  if (form === 'file' || form === 'package') {
    return excerptOf(origin.path);
  }
  const lines = (SOURCES[origin.path] ?? '').split('\n');
  const indent = indentOf(lines[origin.line - 1] ?? '');
  let first = origin.line;
  while (first > 1 && (lines[first - 2] ?? '').trimStart().startsWith('@') && indentOf(lines[first - 2] ?? '') === indent) {
    first -= 1;
  }
  let last = origin.line;
  for (let index = origin.line; index < lines.length; index += 1) {
    const line = lines[index] ?? '';
    if (line.trim() !== '' && indentOf(line) <= indent) {
      break;
    }
    if (line.trim() !== '') {
      last = index + 1;
    }
  }
  return excerptOf(origin.path, first, last);
}

/** The first line of a declaration as the source writes it. */
export function declarationLine(id: string): string {
  const { origin } = raw(id);
  return ((SOURCES[origin.path] ?? '').split('\n')[origin.line - 1] ?? '').trim();
}
