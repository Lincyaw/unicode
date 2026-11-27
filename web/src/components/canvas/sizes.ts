import type { Node, ViewNode } from '../../api/types';

import { fieldParts, opLine, relative, scopeOf, typeParts, type TypePart } from '../marks/format';
import { CODE_SIZE, monoWidth, MONO_SIZE } from './measure';
import { ICON_SMALL } from '../../ui/icons';

/*
 * The sizes of the boxes the node-and-edge views draw, worked out from their content so the layout can place them
 * before they are drawn; cards.tsx draws them at these sizes.
 */

export const CARD_WIDTH = 228;
const BORDER = 1;
const PADDING = 12;
const HEAD = 18;
const ROW = 18;
const GAP = 8;
const NAMES_ROW = 17;
const NAMES_GAP = 10;
const NAME_ICON = 16;

/** At most this many fields a card lists; the rest fold into +n (docs/design.md §7.5). */
export const MAX_FIELDS = 6;

/** The height of a card with a head and some rows. */
export function cardHeight(rows: number): number {
  return 2 * BORDER + 2 * PADDING + HEAD + rows * (ROW + GAP);
}

/** A row of a type card: a field with its type, the references in it apart; an enum value; an operation's signature; or how many more fields the card folds. */
export type CardRow =
  | { kind: 'field'; name: string; type: TypePart[]; warn: boolean }
  | { kind: 'value'; name: string; warn: boolean }
  | { kind: 'op'; op: Node; text: string }
  | { kind: 'more'; count: number };

/** A code type reads relative to the component of the design type it realizes. */
function scopeOfType(node: ViewNode): string {
  return scopeOf(node.bindings?.[0]?.node.id ?? node.id);
}

/** A type's fields or enum values, compared with the code where the view says so. */
function memberRows(node: ViewNode): CardRow[] {
  const scope = scopeOfType(node);
  return node.members
    ? node.members.map(member => {
      const warn = member.status === 'contradicted' || member.status === 'missing';
      return member.kind === 'value'
        ? { kind: 'value', name: member.name, warn }
        : { kind: 'field', name: member.name, type: typeParts(member.type ?? member.codeType ?? '', scope, member.refers), warn };
    })
    : [
      ...(node.attrs.fields ?? []).map((field): CardRow => {
        const { name, type } = fieldParts(field);
        return { kind: 'field', name, type: typeParts(type, scope, node.refers), warn: false };
      }),
      ...(node.attrs.values ?? []).map((name): CardRow => ({ kind: 'value', name, warn: false })),
    ];
}

/** A type's operations, each with its signature; a method's title carries its type's name, which the card's head already gives. */
function opRows(node: ViewNode): CardRow[] {
  const scope = scopeOfType(node);
  return (node.contents ?? []).filter(child => child.kind === 'op').map((op): CardRow => {
    const title = op.title.startsWith(`${node.title}.`) ? op.title.slice(node.title.length + 1) : op.title;
    return { kind: 'op', op, text: relative(opLine({ ...op, title }), scope) };
  });
}

/** The rows of a type's card: its fields or enum values, past MAX_FIELDS folded into +n, then its operations. */
export function typeRows(node: ViewNode): CardRow[] {
  const members = memberRows(node);
  const shown = members.length > MAX_FIELDS ? [...members.slice(0, MAX_FIELDS), { kind: 'more' as const, count: members.length - MAX_FIELDS }] : members;
  return [...shown, ...opRows(node)];
}

/** The rows of a type's card that the reader unfolded with its +n: every field or value, then its operations. */
export function unfoldedTypeRows(node: ViewNode): CardRow[] {
  return [...memberRows(node), ...opRows(node)];
}

/** How tall a folded container's card is: its head and its type names, wrapped in its width. */
export function foldedHeight(names: string[], width = CARD_WIDTH): number {
  if (names.length === 0) {
    return 2 * BORDER + 2 * PADDING + HEAD;
  }
  const room = width - 2 * (BORDER + PADDING);
  let lines = 1;
  let used = 0;
  for (const name of names) {
    const item = NAME_ICON + monoWidth(name, MONO_SIZE);
    if (used > 0 && used + NAMES_GAP + item > room) {
      lines += 1;
      used = item;
    } else {
      used += (used > 0 ? NAMES_GAP : 0) + item;
    }
  }
  return 2 * BORDER + 2 * PADDING + HEAD + GAP + lines * NAMES_ROW + (lines - 1) * GAP;
}

/** How tall an external system's card is. */
export function externalHeight(node: ViewNode): number {
  return cardHeight((node.contents ?? []).filter(child => child.kind === 'op').length);
}

/** The height of a container's head and the gap under it, where the box's content starts. */
export const CONTAINER_HEAD = PADDING + HEAD + PADDING;
export const CONTAINER_PADDING = PADDING;

/** The width of a chip (the prototype's node chip) for a node's name, with or without its status dot. */
export function chipWidth(label: string, dot = true): number {
  return 2 * BORDER + 2 * 8 + (dot ? 8 + 8 : 0) + ICON_SMALL + 8 + monoWidth(label, CODE_SIZE);
}

export const CHIP_HEIGHT = 30;

