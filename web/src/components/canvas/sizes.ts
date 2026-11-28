import type { Node, ViewNode } from '../../api/types';
import { ICON_SMALL } from '../../ui/icons';
import { fieldParts, opLine, relative, scopeOf, typeParts, type TypePart } from '../marks/format';
import { CODE_SIZE, monoWidth, MONO_SIZE } from './measure';

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

/**
 * The lines a folded container's card sets the names it holds on, filled in order within the card's width. The card
 * draws these lines and no others, so its height holds; a name the face draws wider than measured is cut short in its
 * line rather than wrapping past the card.
 */
export function foldedLines<T extends { title: string }>(names: T[], width = CARD_WIDTH): T[][] {
  const room = width - 2 * (BORDER + PADDING);
  const lines: T[][] = [];
  let used = 0;
  for (const name of names) {
    const item = NAME_ICON + monoWidth(name.title, MONO_SIZE);
    const line = lines.at(-1);
    if (line !== undefined && used + NAMES_GAP + item <= room) {
      line.push(name);
      used += NAMES_GAP + item;
    } else {
      lines.push([name]);
      used = item;
    }
  }
  return lines;
}

/** How tall a folded container's card is: its head and the lines of its names. */
export function foldedHeight(names: { title: string }[], width = CARD_WIDTH): number {
  const lines = foldedLines(names, width).length;
  return 2 * BORDER + 2 * PADDING + HEAD + (lines > 0 ? GAP + lines * NAMES_ROW + (lines - 1) * GAP : 0);
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

/** The room a count, such as how many edges a merged edge stands for, takes after the chip it follows. */
export const COUNT_ROOM = 72;

/** The box a count takes after a chip. */
export function countBox(chip: { x: number; y: number; width: number; height: number }): { x: number; y: number; width: number; height: number } {
  return { x: chip.x + chip.width, y: chip.y, width: COUNT_ROOM, height: chip.height };
}

