import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ICON_SMALL } from '../../ui/icons';
import { planChannel, portFor, portRoom, portsAlong, routeChannel, type ChannelLine, type Wall } from '../canvas/channels';
import type { Rect, Route } from '../canvas/layout';
import { monoWidth } from '../canvas/measure';
import { CHIP_HEIGHT, chipWidth } from '../canvas/sizes';
import { shortPath } from '../marks/format';
import { edgeKey, markOf } from './graphParts';

/*
 * Where the code view (Figma 11) draws what unicode serve gives it: the input (a file or a type) with what it contains
 * along contains, and the functions at the other end of its calls, each with the file it is in, or merged into a folded
 * component. Every call the view gives has an end in the input. The input stands in the middle; the functions calling
 * into it stand to its left and those it calls to its right, grouped by their file (docs/design.md §7.5.1); a group the
 * input both calls and is called by stands right. Each line leaves its row at a height of its own and runs through the
 * channel between two columns on a track of its own (channels.ts), so no stretch of one line lies on another; a mark
 * on a line stands in a column beside the wall of its end outside the input, clear of the other marks.
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

/** A row of a box: a type or a function, indented under its type, at its place in the box. */
export interface Row {
  node: ViewNode;
  depth: number;
  rect: Rect;
}

/**
 * A box of the drawing: the input; a file of other functions, headed by a link to the file (`file`, at `path`); or a
 * folded component, a chip centred in the height its lines need.
 */
export interface Box {
  key: string;
  input?: ViewNode;
  file?: { id: string; path: string };
  folded?: ViewNode;
  rows: Row[];
  details?: Rect;
  width: number;
  height: number;
}

export interface CodeLayout {
  input: Box;
  inputNode: ViewNode;
  boxes: { box: Box; rect: Rect }[];
  calls: ViewEdge[];
  routes: Map<string, Route>;
  width: number;
  height: number;
}

/** The name a row shows: a method without its type's name, which the row above already gives. */
export function rowLabel(node: ViewNode, depth: number): string {
  return depth > 0 ? node.title.slice(node.title.lastIndexOf('.') + 1) : node.title;
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

/** Where a line ends: a row of a box, or a folded component, at its height in the drawing, and its box. */
interface End {
  box: Box;
  top: number;
  height: number;
}

/** A drawn call and where it leaves and arrives. */
interface Leg {
  edge: ViewEdge;
  from: End;
  to: End;
  channel: Channel;
}

/** A node of the view by its identity; the view gives every node its edges name. */
function nodeIn(nodes: ReadonlyMap<string, ViewNode>, id: string): ViewNode {
  const node = nodes.get(id);
  if (!node) {
    throw new Error(`the code view names ${id}, which is not one of its nodes`);
  }
  return node;
}

/** The input's rows, the calls, and the other ends of the calls by group: a folded component, or a file. */
function select(view: View) {
  const nodes = new Map(view.nodes.map(node => [node.id, node]));
  const folded = new Set(view.folded);
  const contained = Map.groupBy(view.edges.filter(edge => edge.kind === 'contains'), edge => edge.from);
  const childrenOf = (id: string) => bySource((contained.get(id) ?? []).map(edge => nodeIn(nodes, edge.to)));
  const inputNode = nodeIn(nodes, view.input.id);
  const items = childrenOf(inputNode.id).flatMap(node => [{ node, depth: 0 }, ...childrenOf(node.id).map(child => ({ node: child, depth: 1 }))]);
  const inside = new Set(items.map(item => item.node.id));
  const calls = view.edges.filter(edge => edge.kind === 'calls' || edge.kind === 'reaches');
  const groupOf = (id: string) => {
    if (folded.has(id)) {
      return `folded ${id}`;
    }
    const file = nodeIn(nodes, id).file;
    if (file === undefined) {
      throw new Error(`the code view gives ${id} without the file it is in`);
    }
    return `file ${file}`;
  };
  const members = new Map<string, ViewNode[]>();
  const called = new Set<string>();
  for (const edge of calls) {
    const outside = [edge.from, edge.to].filter(id => !inside.has(id));
    if (outside.length === 2) {
      throw new Error(`the code view draws ${edgeKey(edge)}, which has no end in its input`);
    }
    for (const id of outside) {
      const group = groupOf(id);
      members.set(group, [...new Set([...(members.get(group) ?? []), nodeIn(nodes, id)])]);
      if (id === edge.to) {
        called.add(group);
      }
    }
  }
  const channelOf = (edge: ViewEdge): Channel => ([edge.from, edge.to].some(id => !inside.has(id) && called.has(groupOf(id))) ? 'right' : 'left');
  return { inputNode, items, calls, members, called, channelOf };
}

/** Lays the code view out. */
export function layOutCode(view: View): CodeLayout {
  const { inputNode, items, calls, members, called, channelOf } = select(view);
  // A row (or a folded component) is as tall as the lines at its busiest wall need.
  const ports = new Map<string, number>();
  for (const edge of calls) {
    for (const id of [edge.from, edge.to]) {
      const key = `${id} ${channelOf(edge)}`;
      ports.set(key, (ports.get(key) ?? 0) + 1);
    }
  }
  const heightOf = (id: string, least: number) => Math.max(least, portRoom(Math.max(ports.get(`${id} left`) ?? 0, ports.get(`${id} right`) ?? 0)));
  const input: Box = { ...boxOf(inputNode.id, inputNode.title, items, id => heightOf(id, ROW), inputNode.details ?? 0), input: inputNode };
  const groups: Box[] = [...members].sort(([a], [b]) => (a < b ? -1 : 1)).map(([key, group]): Box => {
    const [first] = group;
    if (first === undefined) {
      throw new Error(`the code view's group ${key} is empty`);
    }
    if (key.startsWith('folded ')) {
      const width = chipWidth(first.title);
      const height = heightOf(first.id, CHIP_HEIGHT);
      return { key, folded: first, rows: [{ node: first, depth: 0, rect: { x: 0, y: 0, width, height } }], width, height };
    }
    const file = { id: key.slice('file '.length), path: first.origin.path };
    return { ...boxOf(key, shortPath(file.path), bySource(group).map(node => ({ node, depth: 0 })), id => heightOf(id, ROW), 0), file };
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
  const topOf = (box: Box) => {
    const top = tops.get(box);
    if (top === undefined) {
      throw new Error(`the code view placed no column for ${box.key}`);
    }
    return top;
  };
  const boxes = [input, ...groups];
  const rows = new Map(boxes.flatMap(box => box.rows.map(row => [row.node.id, { box, top: topOf(box) + row.rect.y, height: row.rect.height }] as const)));
  const endOf = (id: string): End => {
    const end = rows.get(id);
    if (!end) {
      throw new Error(`the code view draws a call to ${id}, which has no row`);
    }
    // Each end of a call is its own, so a function calling itself leaves and enters its row at two heights.
    return { ...end };
  };
  const legs: Leg[] = calls.map(edge => ({ edge, from: endOf(edge.from), to: endOf(edge.to), channel: channelOf(edge) }));
  const channelLines = (channel: Channel) => {
    const own = legs.filter(leg => leg.channel === channel);
    return own.map(leg => lineOf(leg, own, input));
  };
  const leftLines = channelLines('left');
  const rightLines = channelLines('right');
  const leftPlan = planChannel(leftLines, leftLines.length > 0 ? CHANNEL_MIN : 0);
  const rightPlan = planChannel(rightLines, rightLines.length > 0 ? CHANNEL_MIN : 0);
  // A left box stands flush with the channel.
  const leftWidth = Math.max(0, ...left.map(box => box.width));
  const inputX = leftWidth + leftPlan.width;
  const rightX = inputX + input.width + rightPlan.width;
  const placed = boxes.map(box => {
    const x = box === input ? inputX : left.includes(box) ? leftWidth - box.width : rightX;
    return { box, rect: { x, y: topOf(box), width: box.width, height: box.height } };
  });
  return {
    input,
    inputNode,
    boxes: placed,
    calls,
    routes: new Map([...routeChannel(leftLines, leftPlan, leftWidth), ...routeChannel(rightLines, rightPlan, inputX + input.width)]),
    width: Math.max(0, ...placed.map(({ rect }) => rect.x + rect.width)),
    height: Math.max(0, ...placed.map(({ rect }) => rect.y + rect.height)),
  };
}

/**
 * A leg as a line of its channel: it leaves and arrives at the walls its boxes face. Each end takes a height of its own
 * in its row, on the row's ports for that wall (channels.ts's portsAlong), the lines of a row ordered by where their other
 * ends are so they cross as little as they can.
 */
function lineOf(leg: Leg, legs: Leg[], input: Box): ChannelLine {
  const wallOf = (end: End): Wall => ((end.box === input) === (leg.channel === 'left') ? 'end' : 'start');
  // A mark stands beside the wall of the line's end outside the input; a call within the input has it beside the input.
  const outside = leg.from.box === input ? leg.to : leg.from;
  const mark = markOf(leg.edge);
  const heightAt = (end: End) => {
    const sharing = legs.flatMap(other => [other.from, other.to].filter(item => item.box === end.box && item.top === end.top).map(item => ({ other, item })));
    const centre = (other: Leg, item: End) => {
      const far = item === other.from ? other.to : other.from;
      return far.top + far.height / 2;
    };
    const ordered = sharing.sort((a, b) => centre(a.other, a.item) - centre(b.other, b.item));
    const index = ordered.findIndex(({ other, item }) => other === leg && item === end);
    return portFor(index, ordered.length, portsAlong(end.top, end.top + end.height, wallOf(end)));
  };
  return {
    id: edgeKey(leg.edge), from: { y: heightAt(leg.from), wall: wallOf(leg.from) }, to: { y: heightAt(leg.to), wall: wallOf(leg.to) },
    ...(mark ? { mark: { wall: wallOf(outside), ...mark } } : {}),
  };
}

