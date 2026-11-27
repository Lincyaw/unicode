import type { Entity, NodeChange } from '../../api/types';
import { CHANGE_WORDS } from '../../terms';
import { fieldParts } from '../marks/format';

/** How the node and the code that realizes it changed since the base, said in words. */
export function changeWords(entity: Entity): string[] {
  const changes = entity.changes ?? [];
  const own = changes.find(change => change.id === entity.node.id);
  const code = changes.filter(change => change.id !== entity.node.id);
  if (own?.change === 'added') {
    return [CHANGE_WORDS.added];
  }
  if (own?.change === 'removed') {
    return [CHANGE_WORDS.removed];
  }
  const words = (own?.attrs ?? []).flatMap(change => attrWords(change));
  if (entity.node.kind === 'op') {
    const bodies = code.filter(change => change.change === 'modified' && (change.linesChanged ?? 0) > 0);
    if (bodies.length > 0 && !words.includes(CHANGE_WORDS.signatureChanged)) {
      words.push(CHANGE_WORDS.signatureKept);
    }
    words.push(...bodies.map(change => CHANGE_WORDS.bodyChanged(change.linesChanged ?? 0)));
  }
  return words;
}

function attrWords(change: NodeChange['attrs'][number]): string[] {
  switch (change.key) {
    case 'fields':
      return [...change.added.map(field => CHANGE_WORDS.addedField(fieldParts(field).name)), ...change.removed.map(field => CHANGE_WORDS.removedField(fieldParts(field).name))];
    case 'values':
      return [...change.added.map(CHANGE_WORDS.addedValue), ...change.removed.map(CHANGE_WORDS.removedValue)];
    case 'signature':
      return [CHANGE_WORDS.signatureChanged];
    default:
      return [CHANGE_WORDS.attrChanged(change.key)];
  }
}

