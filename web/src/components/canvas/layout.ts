import { Graph, layout } from '@dagrejs/dagre';

/*
 * Where the boxes of a node-and-edge view stand, and how the lines between them run. dagre ranks and orders the boxes;
 * the lines are then drawn orthogonally between the boxes' facing sides, as the prototype draws them: straight where
 * two boxes overlap across the gap, with one bend at the middle of the gap where they do not.
 */

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Box {
  id: string;
  width: number;
  height: number;
}

export interface Link {
  id: string;
  from: string;
  to: string;

  /** Drawn but left out of the ranking, such as an undeclared dependency that runs against the design's direction */
  unranked?: boolean;
}

export type Direction = 'TB' | 'LR';

export interface Arrangement {
  rects: Map<string, Rect>;

  /** For a link across several ranks, the points it passes between the boxes of the ranks it crosses */
  bends: Map<string, { x: number; y: number }[]>;

  width: number;
  height: number;
}

/** The gaps between boxes in a rank and between ranks, by how the view reads. */
export interface Spacing {
  node: number;
  rank: number;
}

export const ROOMY: Spacing = { node: 36, rank: 64 };
export const TIGHT: Spacing = { node: 16, rank: 40 };

/**
 * Places the boxes with dagre; the arrangement starts at the origin. The boxes named in `beside` stay out of the
 * ranking and stand in a column to the right of the box each maps to, top-aligned with it, as an external system
 * beside the component that uses it; the box and its column are ranked as one, so nothing comes between them.
 */
export function arrange(boxes: Box[], links: Link[], direction: Direction, spacing: Spacing, beside: ReadonlyMap<string, string> = new Map()): Arrangement {
  const byId = new Map(boxes.map(box => [box.id, box]));
  const anchorOf = (id: string) => {
    const anchor = beside.get(id);
    return anchor !== undefined && byId.has(anchor) && !beside.has(anchor) ? anchor : undefined;
  };
  const columns = Map.groupBy(boxes.filter(box => anchorOf(box.id) !== undefined), box => anchorOf(box.id) ?? '');
  const columnSize = (items: Box[]) => ({
    width: Math.max(...items.map(item => item.width)),
    height: items.reduce((sum, item) => sum + item.height, 0) + (items.length - 1) * spacing.node / 2,
  });
  const ranked = boxes.filter(box => anchorOf(box.id) === undefined).map(box => {
    const column = columns.get(box.id);
    if (!column) {
      return box;
    }
    const size = columnSize(column);
    return { ...box, width: box.width + spacing.node + size.width, height: Math.max(box.height, size.height) };
  });
  const lift = (id: string) => anchorOf(id) ?? id;
  const rankedLinks = links.filter(link => !link.unranked).map(link => ({ ...link, from: lift(link.from), to: lift(link.to) }));
  const placed = arrangeRanked(ranked, rankedLinks, direction, spacing);
  for (const [anchorId, column] of columns) {
    const group = placed.rects.get(anchorId);
    const anchor = byId.get(anchorId);
    if (!group || !anchor) {
      continue;
    }
    placed.rects.set(anchorId, { ...group, width: anchor.width, height: anchor.height });
    const x = group.x + anchor.width + spacing.node;
    let y = group.y;
    for (const box of column) {
      placed.rects.set(box.id, { x, y, width: box.width, height: box.height });
      y += box.height + spacing.node / 2;
    }
  }
  return placed;
}

function arrangeRanked(boxes: Box[], links: Link[], direction: Direction, spacing: Spacing): Arrangement {
  const graph = new Graph({ multigraph: true });
  graph.setGraph({ rankdir: direction, nodesep: spacing.node, ranksep: spacing.rank, marginx: 0, marginy: 0 });
  graph.setDefaultEdgeLabel(() => ({}));
  for (const box of boxes) {
    graph.setNode(box.id, { width: box.width, height: box.height });
  }
  const known = new Set(boxes.map(box => box.id));
  for (const link of links) {
    if (known.has(link.from) && known.has(link.to) && link.from !== link.to) {
      graph.setEdge(link.from, link.to, {}, link.id);
    }
  }
  layout(graph);
  const rects = new Map<string, Rect>();
  for (const box of boxes) {
    const placed = graph.node(box.id);
    rects.set(box.id, { x: placed.x - box.width / 2, y: placed.y - box.height / 2, width: box.width, height: box.height });
  }
  // A link that spans several ranks keeps the points dagre threads between the boxes of the ranks it crosses.
  const bends = new Map<string, { x: number; y: number }[]>();
  for (const edge of graph.edges()) {
    const points = graph.edge(edge)?.points ?? [];
    if (points.length > 3 && edge.name !== undefined) {
      bends.set(edge.name, points.slice(1, -1));
    }
  }
  const shift = direction === 'TB' ? centerRanks(rects) : new Map<number, number>();
  for (const [id, points] of bends) {
    bends.set(id, points.map(point => ({ x: point.x + (shift.get(nearestRank(shift, point.y)) ?? 0), y: point.y })));
  }
  // dagre may thread a line left of every box; the drawing then moves right so the line stays on it.
  const left = Math.min(0, ...[...bends.values()].flatMap(points => points.map(point => point.x - BEND_ROOM)));
  if (left < 0) {
    for (const [id, rect] of rects) {
      rects.set(id, { ...rect, x: rect.x - left });
    }
    for (const [id, points] of bends) {
      bends.set(id, points.map(point => ({ ...point, x: point.x - left })));
    }
  }
  return {
    rects,
    bends,
    width: Math.max(0, ...[...rects.values()].map(rect => rect.x + rect.width), ...[...bends.values()].flatMap(points => points.map(point => point.x + BEND_ROOM))),
    height: Math.max(0, ...[...rects.values()].map(rect => rect.y + rect.height)),
  };
}

/** How far a threaded line keeps from the drawing's edge. */
const BEND_ROOM = 8;

function nearestRank(shift: ReadonlyMap<number, number>, y: number): number {
  let best = y;
  let distance = Infinity;
  for (const rank of shift.keys()) {
    if (Math.abs(rank - y) < distance) {
      distance = Math.abs(rank - y);
      best = rank;
    }
  }
  return best;
}

/** How much wider than its widest rank a drawing may come out before its ranks are centred on one another. */
const SLACK = 24;

/**
 * dagre can push a wide box off to one side of the ranks under it, which leaves the drawing wider than any rank needs;
 * then every rank is centred on the widest instead.
 */
function centerRanks(rects: Map<string, Rect>): Map<number, number> {
  const shifts = new Map<number, number>();
  const ranks = Map.groupBy([...rects.entries()], ([, rect]) => Math.round(rect.y + rect.height / 2));
  const spans = [...ranks.values()].map(rank => ({
    rank,
    left: Math.min(...rank.map(([, rect]) => rect.x)),
    right: Math.max(...rank.map(([, rect]) => rect.x + rect.width)),
  }));
  const widest = Math.max(...spans.map(span => span.right - span.left));
  const total = Math.max(...spans.map(span => span.right)) - Math.min(...spans.map(span => span.left));
  if (total <= widest + SLACK) {
    return shifts;
  }
  for (const span of spans) {
    const shift = (widest - (span.right - span.left)) / 2 - span.left;
    for (const [id, rect] of span.rank) {
      rects.set(id, { ...rect, x: rect.x + shift });
      shifts.set(Math.round(rect.y + rect.height / 2), shift);
    }
  }
  return shifts;
}

/** Moves every rectangle by an offset. */
export function shifted(rects: Map<string, Rect>, dx: number, dy: number): Map<string, Rect> {
  return new Map([...rects].map(([id, rect]) => [id, { ...rect, x: rect.x + dx, y: rect.y + dy }]));
}

/** Moves the bends of links by an offset, with the boxes they run between. */
export function shiftedBends(bends: ReadonlyMap<string, { x: number; y: number }[]>, dx: number, dy: number): Map<string, { x: number; y: number }[]> {
  return new Map([...bends].map(([id, points]) => [id, points.map(point => ({ x: point.x + dx, y: point.y + dy }))]));
}

export interface Point {
  x: number;
  y: number;
}

/** A line between two boxes: the corners it runs through, its path, and the point where its label or badge sits. */
export interface Route {
  points: Point[];
  d: string;
  mid: Point;
}

/** The point a fraction of the way along a line through the points, and whether the stretch it lies on runs down. */
export function pointAlong(points: Point[], fraction: number): { point: Point; down: boolean } {
  const lengths = points.slice(1).map((point, index) => Math.hypot(point.x - (points[index]?.x ?? point.x), point.y - (points[index]?.y ?? point.y)));
  let rest = lengths.reduce((sum, length) => sum + length, 0) * fraction;
  for (const [index, length] of lengths.entries()) {
    const start = points[index];
    const end = points[index + 1];
    if (start && end && (rest <= length || index === lengths.length - 1)) {
      const t = length === 0 ? 0 : Math.min(1, rest / length);
      return { point: { x: start.x + (end.x - start.x) * t, y: start.y + (end.y - start.y) * t }, down: start.x === end.x };
    }
    rest -= length;
  }
  return { point: points[0] ?? { x: 0, y: 0 }, down: false };
}

/** The line through the points, each joined to the next straight across or down; its label sits at `mid`, by default half-way along. */
export function routeThrough(points: Point[], mid: Point = pointAlong(points, 0.5).point): Route {
  const [first, ...rest] = points;
  let d = first ? `M ${first.x} ${first.y}` : '';
  let at = first;
  for (const point of rest) {
    d += at && point.x === at.x ? ` V ${point.y}` : at && point.y === at.y ? ` H ${point.x}` : ` L ${point.x} ${point.y}`;
    at = point;
  }
  return { points, d, mid };
}

/** The least overlap, across the gap, for a line to run straight between two boxes. */
const STRAIGHT_OVERLAP = 24;

function overlap(a0: number, a1: number, b0: number, b1: number): [number, number] | undefined {
  const start = Math.max(a0, b0);
  const end = Math.min(a1, b1);
  return end - start >= STRAIGHT_OVERLAP ? [start, end] : undefined;
}

/** Where along a shared span the `slot`-th of `slots` parallel lines runs. */
function along([start, end]: [number, number], slot: number, slots: number): number {
  return start + ((end - start) * (slot + 1)) / (slots + 1);
}

/**
 * An orthogonal line from one box to another. Boxes apart vertically connect bottom to top, boxes apart horizontally
 * right to left; parallel lines between the same two boxes take separate slots along their shared span.
 */
export function route(from: Rect, to: Rect, slot = 0, slots = 1, direction: Direction = 'TB'): Route {
  if (direction === 'LR' && to.x >= from.x + from.width) {
    // A left-to-right drawing leaves from the right side and enters from the left, bending once at the gap's middle.
    const x1 = from.x + from.width;
    const x2 = to.x;
    const y1 = along([from.y, from.y + from.height], slot, slots);
    const y2 = to.y + to.height / 2;
    const xMid = x1 + Math.min((x2 - x1) / 2, 80);
    return y1 === y2
      ? routeThrough([{ x: x1, y: y1 }, { x: x2, y: y1 }])
      : routeThrough([{ x: x1, y: y1 }, { x: xMid, y: y1 }, { x: xMid, y: y2 }, { x: x2, y: y2 }], { x: (xMid + x2) / 2, y: y2 });
  }
  const below = to.y >= from.y + from.height;
  const above = to.y + to.height <= from.y;
  const right = to.x >= from.x + from.width;
  const left = to.x + to.width <= from.x;
  if (below || above) {
    const y1 = below ? from.y + from.height : from.y;
    const y2 = below ? to.y : to.y + to.height;
    const span = overlap(from.x, from.x + from.width, to.x, to.x + to.width);
    if (span) {
      const x = along(span, slot, slots);
      return routeThrough([{ x, y: y1 }, { x, y: y2 }]);
    }
    if (right || left) {
      // Leave from the side facing the other box and come down or up into it: one bend.
      const x1 = right ? from.x + from.width : from.x;
      const y = along([from.y, from.y + from.height], slot, slots);
      const x2 = to.x + to.width / 2;
      return routeThrough([{ x: x1, y }, { x: x2, y }, { x: x2, y: y2 }], { x: (x1 + x2) / 2, y });
    }
    const x1 = from.x + from.width / 2;
    const x2 = to.x + to.width / 2;
    const yMid = (y1 + y2) / 2;
    return routeThrough([{ x: x1, y: y1 }, { x: x1, y: yMid }, { x: x2, y: yMid }, { x: x2, y: y2 }], { x: (x1 + x2) / 2, y: yMid });
  }
  const x1 = right ? from.x + from.width : from.x;
  const x2 = right ? to.x : to.x + to.width;
  const span = overlap(from.y, from.y + from.height, to.y, to.y + to.height);
  if (span) {
    const y = along(span, slot, slots);
    return routeThrough([{ x: x1, y }, { x: x2, y }]);
  }
  const y1 = from.y + from.height / 2;
  const y2 = to.y + to.height / 2;
  const xMid = (x1 + x2) / 2;
  return routeThrough([{ x: x1, y: y1 }, { x: xMid, y: y1 }, { x: xMid, y: y2 }, { x: x2, y: y2 }], { x: xMid, y: (y1 + y2) / 2 });
}

/**
 * A line down (or up) through the points dagre left between the ranks it crosses, drawn orthogonally: straight down
 * where the points line up, bending half-way between two points where they do not.
 */
function threaded(from: Rect, to: Rect, through: Point[]): Route {
  const down = to.y >= from.y;
  const clamp = (x: number, rect: Rect) => Math.min(rect.x + rect.width - 8, Math.max(rect.x + 8, x));
  const first = through[0] ?? { x: from.x + from.width / 2, y: from.y };
  const last = through.at(-1) ?? first;
  const stops = [
    { x: clamp(first.x, from), y: down ? from.y + from.height : from.y },
    ...through,
    { x: clamp(last.x, to), y: down ? to.y : to.y + to.height },
  ];
  const points: Point[] = stops.slice(0, 1);
  for (const [index, stop] of stops.slice(1).entries()) {
    const previous = stops[index] ?? stop;
    if (Math.abs(stop.x - previous.x) >= 1) {
      const middle = (previous.y + stop.y) / 2;
      points.push({ x: previous.x, y: middle }, { x: stop.x, y: middle });
    }
    points.push(Math.abs(stop.x - previous.x) < 1 ? { x: previous.x, y: stop.y } : stop);
  }
  return routeThrough(points, stops[Math.floor(stops.length / 2)] ?? first);
}

/** Routes every link between placed boxes, giving parallel links between the same pair their own slots. */
export function routeAll(links: Link[], rects: Map<string, Rect>, direction: Direction = 'TB', bends: ReadonlyMap<string, { x: number; y: number }[]> = new Map()): Map<string, Route> {
  const pairs = Map.groupBy(links.filter(link => rects.has(link.from) && rects.has(link.to)), link => [link.from, link.to].sort().join('\u0000'));
  const routes = new Map<string, Route>();
  for (const group of pairs.values()) {
    group.forEach((link, slot) => {
      const from = rects.get(link.from);
      const to = rects.get(link.to);
      const through = bends.get(link.id);
      if (from && to) {
        routes.set(link.id, through && direction === 'TB' ? threaded(from, to, through) : route(from, to, direction === 'LR' ? 0 : slot, direction === 'LR' ? 1 : group.length, direction));
      }
    });
  }
  return routes;
}
