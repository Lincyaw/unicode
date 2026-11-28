import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ICON_SMALL } from '../../ui/icons';
import { planChannel, routeChannel, type ChannelLine, type Wall } from '../canvas/channels';
import type { Rect, Route } from '../canvas/layout';
import { monoWidth } from '../canvas/measure';
import { CHIP_HEIGHT, chipWidth, countBox, COUNT_ROOM } from '../canvas/sizes';
import { shortPath } from '../marks/format';
import { edgeKey } from './graphParts';

/*
 * Where the code view (Figma 11) draws what unicode serve gives it: the input (a file or a type) with what it contains
 * along contains, and the functions that call into it or that it calls, each with its origin, or merged into a folded
 * component. The input stands in the middle; the functions calling into it stand to its left and those it calls to its
 * right, grouped by the file they are in (docs/design.md §7.5.1); a group the input both calls and is called by stands
 * right. Only the calls with an end in the input are drawn. Each line leaves its row at a height of its own and runs
 * through the channel between two columns on a track of its own (channels.ts), so no line lies on another.
 */

const BOX_GAP = 20;
const CHANNEL_MIN = 72;
const PADDING = 12;
const HEAD = 18;
const ROW = 30;
const GAP = 6;
const INDENT = 16;
const BINDING_GAP = 24;
const MIN_WIDTH = 240;

/** How far apart the lines leaving or entering one row run; a row with many lines grows to keep them apart. */
const PORT_GAP = 4;

/** A row of a box: a type or a function, indented under its type, at its place in the box. */
export interface Row {
  node: ViewNode;
  depth: number;
  rect: Rect;
}

/**
 * A box of the drawing: the input, a file of other functions (`path`), or a folded component, a chip with its count
 * centred in the height its lines need.
 */
export interface Box {
  key: string;
  file?: ViewNode;
  path?: string;
  folded?: ViewNode;
  rows: Row[];
  details?: Rect;
  width: number;
  height: number;
}

export interface CodeLayout {
  input: Box;
  boxes: Box[];
  place: Map<Box, Rect>;
  calls: ViewEdge[];
  routes: Map<string, Route>;
  obstacles: Rect[];
  width: number;
  height: number;
}

/** The name a row shows: a method without its type's name, which the row above already gives. */
export function rowLabel(node: ViewNode, depth: number): string {
  return depth > 0 ? node.title.split('.').at(-1) ?? node.title : node.title;
}

/** The design node a code element realizes, which its row names. */
export function bindingOf(node: ViewNode): ViewNode | undefined {
  return node.bindings?.[0]?.node;
}

/** Nodes in the order their source declares them. */
function bySource(nodes: ViewNode[]): ViewNode[] {
  return [...nodes].sort((a, b) => a.origin.line - b.origin.line || a.origin.column - b.origin.column || (a.id < b.id ? -1 : 1));
}

/** A box with a head and rows, wide enough for its widest row and its head, with a row for the input's undrawn details. */
function boxOf(key: string, head: string, items: { node: ViewNode; depth: number }[], heightOf: (id: string) => number, details: number): Box {
  const widths = items.map(({ node, depth }) => {
    const bound = bindingOf(node);
    return depth * INDENT + chipWidth(rowLabel(node, depth)) + (bound ? BINDING_GAP + ICON_SMALL + 4 + monoWidth(bound.title, 12) + 16 : 0);
  });
  const width = Math.max(MIN_WIDTH, monoWidth(head, 12) + 60, ...widths.map(item => item + 2 * PADDING));
  let y = PADDING + HEAD + GAP + 4;
  const rows = items.map(({ node, depth }) => {
    const rect = { x: PADDING + depth * INDENT, y, width: width - 2 * PADDING - depth * INDENT, height: heightOf(node.id) };
    y += rect.height + GAP;
    return { node, depth, rect };
  });
  const detailsRect = details > 0 ? { x: PADDING + INDENT, y, width: 80, height: CHIP_HEIGHT - 8 } : undefined;
  if (detailsRect) {
    y += detailsRect.height + GAP;
  }
  return { key, rows, details: detailsRect, width, height: y + PADDING - GAP };
}

/** The channel a call runs through: the left one, unless an end is in a group right of the input. */
type Channel = 'left' | 'right';

/** Where a line ends: a row of a box, or a folded component, and its box. */
interface End {
  box: Box;
  rect: Rect;
}

/** A drawn call and where it leaves and arrives. */
interface Leg {
  edge: ViewEdge;
  from: End;
  to: End;
  channel: Channel;
}

/** The input's rows, the calls with an end in them, and the other ends of those calls, by group. */
function select(view: View) {
  const nodes = new Map(view.nodes.map(node => [node.id, node]));
  const folded = new Set(view.folded);
  const contained = Map.groupBy(view.edges.filter(edge => edge.kind === 'contains'), edge => edge.from);
  const childrenOf = (id: string) => bySource((contained.get(id) ?? []).flatMap(edge => nodes.get(edge.to) ?? []));
  const inputNode: ViewNode = nodes.get(view.input.id) ?? view.input;
  const items = childrenOf(inputNode.id).flatMap(node => [{ node, depth: 0 }, ...childrenOf(node.id).map(child => ({ node: child, depth: 1 }))]);
  const inside = new Set(items.map(item => item.node.id));
  const calls = view.edges.filter(edge => (edge.kind === 'calls' || edge.kind === 'reaches') && (inside.has(edge.from) || inside.has(edge.to)));
  const groupOf = (id: string) => (folded.has(id) ? `folded ${id}` : `file ${nodes.get(id)?.origin.path ?? id}`);
  const members = new Map<string, Set<string>>();
  const called = new Set<string>();
  for (const edge of calls) {
    for (const end of [edge.from, edge.to].filter(id => !inside.has(id))) {
      members.set(groupOf(end), (members.get(groupOf(end)) ?? new Set()).add(end));
      if (end === edge.to) {
        called.add(groupOf(end));
      }
    }
  }
  const channelOf = (edge: ViewEdge): Channel => ([edge.from, edge.to].some(id => !inside.has(id) && called.has(groupOf(id))) ? 'right' : 'left');
  return { nodes, inputNode, items, calls, members, called, channelOf };
}

/** Lays the code view out. */
export function layOutCode(view: View): CodeLayout {
  const { nodes, inputNode, items, calls, members, called, channelOf } = select(view);
  // A row (or a folded component) is as tall as the lines at its busiest wall need.
  const ports = new Map<string, number>();
  for (const edge of calls) {
    for (const id of [edge.from, edge.to]) {
      const key = `${id} ${channelOf(edge)}`;
      ports.set(key, (ports.get(key) ?? 0) + 1);
    }
  }
  const heightOf = (id: string, least: number) => Math.max(least, ((Math.max(ports.get(`${id} left`) ?? 0, ports.get(`${id} right`) ?? 0)) + 1) * PORT_GAP);
  const input: Box = { ...boxOf(inputNode.id, inputNode.title, items, id => heightOf(id, ROW), inputNode.details ?? 0), file: inputNode };
  const groups: Box[] = [...members].sort(([a], [b]) => (a < b ? -1 : 1)).map(([key, ids]) => {
    const component = key.startsWith('folded ') ? nodes.get([...ids][0] ?? '') : undefined;
    if (component) {
      const width = chipWidth(component.title);
      const height = heightOf(component.id, CHIP_HEIGHT);
      return { key, folded: component, rows: [{ node: component, depth: 0, rect: { x: 0, y: 0, width, height } }], width, height };
    }
    const path = key.slice('file '.length);
    const rows = bySource([...ids].flatMap(id => nodes.get(id) ?? [])).map(node => ({ node, depth: 0 }));
    return { ...boxOf(key, shortPath(path), rows, id => heightOf(id, ROW), 0), path };
  });
  const left = groups.filter(box => !called.has(box.key));
  const right = groups.filter(box => called.has(box.key));
  const tops = new Map<Box, number>([[input, 0]]);
  for (const column of [left, right]) {
    let y = 0;
    for (const box of column) {
      tops.set(box, y);
      y += box.height + BOX_GAP;
    }
  }
  const boxes = [input, ...groups];
  const endOf = (id: string): End | undefined => {
    const box = boxes.find(item => item.rows.some(row => row.node.id === id));
    const row = box?.rows.find(item => item.node.id === id);
    return box && row ? { box, rect: { ...row.rect, y: (tops.get(box) ?? 0) + row.rect.y } } : undefined;
  };
  const legs: Leg[] = calls.flatMap(edge => {
    const from = endOf(edge.from);
    const to = endOf(edge.to);
    return from && to ? [{ edge, from, to, channel: channelOf(edge) }] : [];
  });
  const channelLines = (channel: Channel) => legs.filter(leg => leg.channel === channel).map(leg => lineOf(leg, legs, input));
  const leftLines = channelLines('left');
  const rightLines = channelLines('right');
  const leftPlan = planChannel(leftLines, leftLines.length > 0 ? CHANNEL_MIN : 0);
  const rightPlan = planChannel(rightLines, rightLines.length > 0 ? CHANNEL_MIN : 0);
  // A left box stands flush with the channel; a folded component's count stands between its chip and the channel.
  const leftWidth = Math.max(0, ...left.map(box => box.width + (box.folded ? COUNT_ROOM : 0)));
  const inputX = leftWidth + leftPlan.width;
  const rightX = inputX + input.width + rightPlan.width;
  const placed = boxes.map(box => {
    const x = box === input ? inputX : left.includes(box) ? leftWidth - box.width - (box.folded ? COUNT_ROOM : 0) : rightX;
    return { box, rect: { x, y: tops.get(box) ?? 0, width: box.width, height: box.height } };
  });
  // The marks keep off every box, and off the counts after the folded components' chips.
  const counts = placed.filter(({ box }) => box.folded).map(({ rect }) => countBox(rect));
  return {
    input,
    boxes,
    place: new Map(placed.map(({ box, rect }) => [box, rect])),
    calls: legs.map(leg => leg.edge),
    routes: new Map([...routeChannel(leftLines, leftPlan, leftWidth), ...routeChannel(rightLines, rightPlan, inputX + input.width)]),
    obstacles: [...placed.map(({ rect }) => rect), ...counts],
    width: Math.max(0, ...placed.map(({ rect }) => rect.x + rect.width), ...counts.map(rect => rect.x + rect.width)),
    height: Math.max(0, ...placed.map(({ rect }) => rect.y + rect.height)),
  };
}

/**
 * A leg as a line of its channel: it leaves and arrives at the walls its boxes face, each end at a height of its own in
 * its row, the lines of a row ordered by where their other ends are so they cross as little as they can.
 */
function lineOf(leg: Leg, legs: Leg[], input: Box): ChannelLine {
  const wallOf = (end: End): Wall => ((end.box === input) === (leg.channel === 'left') ? 'end' : 'start');
  const heightAt = (end: End) => {
    const sharing = legs.flatMap(other => (other.channel === leg.channel ? [other.from, other.to].filter(item => item.box === end.box && item.rect.y === end.rect.y).map(item => ({ other, item })) : []));
    const centre = (other: Leg, item: End) => {
      const far = item === other.from ? other.to : other.from;
      return far.rect.y + far.rect.height / 2;
    };
    const ordered = sharing.sort((a, b) => centre(a.other, a.item) - centre(b.other, b.item));
    const index = ordered.findIndex(({ other, item }) => other === leg && item === end);
    return end.rect.y + (end.rect.height * (index + 1)) / (ordered.length + 1);
  };
  return { id: edgeKey(leg.edge), from: { y: heightAt(leg.from), wall: wallOf(leg.from) }, to: { y: heightAt(leg.to), wall: wallOf(leg.to) } };
}
