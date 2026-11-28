import { channelRoom, planChannel, PORT_GRID, portFor, portNear, portRoom, portsAlong, routeChannel, type ChannelLine, type Wall } from './channels';
import { arrange, routeThrough, type Arrangement, type Box, type Direction, type Link, type Point, type Rect, type Route, type Spacing } from './layout';

/*
 * A drawing whose boxes stand in dagre's ranks, with lines that run as the code view's do (channels.ts). layout.ts's
 * arrange ranks, orders and places the boxes; then each line runs from rank to rank. It leaves its box on the side that
 * faces the rank it goes to, at a port of its own, and runs to the edge of its rank; between two neighbouring ranks it
 * crosses their channel on a track of its own; across a rank in between it passes where dagre left it room among that
 * rank's boxes, stepping over by one port step in the rank's middle. A channel's two walls take ports of alternate
 * parity, so no stretch of one line lies on another. A box with more lines on a side than the side holds grows along
 * it, and each channel widens to hold its tracks. A box standing beside another (arrange's `beside`) is joined to it
 * through a channel of its own between the two.
 */

/**
 * A link to draw from `from` to `to`; `reversed` ranks it the other way round, such as a call back into a view's input.
 * A `mark` of this size stands on the line in a column of marks beside the wall next to whichever of its ends meets
 * fewer lines on its side (the end it goes to when they meet as many), as the code view's marks do; the route's `mid`
 * is the mark's centre.
 */
export interface LayeredLink extends Link {
  reversed?: boolean;
  mark?: { width: number; height: number };
}

/**
 * Boxes that stand as one in their rank, one after another across it, each set in from the stack's start by its
 * indent; lines meet each member on its own sides.
 */
export interface Stack {
  id: string;
  members: { id: string; indent: number }[];
}

/** Where everything stands: each box (and each stack), each line's route, and the drawing's size. */
export interface Layered {
  rects: Map<string, Rect>;
  routes: Map<string, Route>;
  width: number;
  height: number;
}

/** How far apart the members of a stack stand. */
const STACK_GAP = 14;

interface Size {
  width: number;
  height: number;
}

/** The two directions of a drawing: along the ranks (main) and across them (cross). */
interface Axis {
  main: (point: Point) => number;
  cross: (point: Point) => number;
  point: (main: number, cross: number) => Point;
  mainStart: (rect: Rect) => number;
  mainSize: (size: Size) => number;
  crossStart: (rect: Rect) => number;
  crossSize: (size: Size) => number;
  rect: (main: number, cross: number, mainSize: number, crossSize: number) => Rect;
}

const AXES: Record<Direction, Axis> = {
  TB: {
    main: point => point.y, cross: point => point.x, point: (main, cross) => ({ x: cross, y: main }),
    mainStart: rect => rect.y, mainSize: size => size.height, crossStart: rect => rect.x, crossSize: size => size.width,
    rect: (main, cross, mainSize, crossSize) => ({ x: cross, y: main, width: crossSize, height: mainSize }),
  },
  LR: {
    main: point => point.x, cross: point => point.y, point: (main, cross) => ({ x: main, y: cross }),
    mainStart: rect => rect.x, mainSize: size => size.width, crossStart: rect => rect.y, crossSize: size => size.height,
    rect: (main, cross, mainSize, crossSize) => ({ x: main, y: cross, width: mainSize, height: crossSize }),
  },
};

/**
 * The side of a box where a line meets it: the one facing the rank before (start: top or left), the one facing the rank
 * after (end: bottom or right), or the one facing the box it stands beside.
 */
type Side = 'start' | 'end' | 'beside';

/**
 * Where one end of a line meets a box: the box, its side, the wall of the channel that side lies on, and the cross
 * position the line heads for from there, which orders the lines of a side.
 */
interface Meet {
  key: string;
  box: string;
  side: Side;
  wall: Wall;
  toward: number;
}

/** A line's stop in one rank on its way: its port at a box, or where it passes between the rank's boxes. */
type Stop = { rank: number; meet: Meet } | { rank: number; pass: number };

/** Groups the ranked boxes by rank: boxes whose ranks' centres lie at one height (TB) or in one column (LR) share a rank. */
function ranksOf(arrangement: Arrangement, ids: string[]): Map<string, number> {
  const centres = ids.map(id => {
    const centre = arrangement.centres.get(id);
    if (centre === undefined) {
      throw new Error(`the layered drawing placed no box for ${id}`);
    }
    return { id, centre };
  }).sort((a, b) => a.centre - b.centre);
  const ranks = new Map<string, number>();
  let rank = -1;
  let last = -Infinity;
  for (const { id, centre } of centres) {
    if (centre > last + 1) {
      rank += 1;
      last = centre;
    }
    ranks.set(id, rank);
  }
  return ranks;
}

function rankIn(ranks: ReadonlyMap<string, number>, id: string): number {
  const rank = ranks.get(id);
  if (rank === undefined) {
    throw new Error(`the layered drawing ranked no box for ${id}`);
  }
  return rank;
}

/** The points of a line without repeats and without points in the middle of a straight run. */
function tidy(points: Point[]): Point[] {
  const kept: Point[] = [];
  for (const point of points) {
    const last = kept.at(-1);
    if (last && Math.abs(last.x - point.x) < 0.01 && Math.abs(last.y - point.y) < 0.01) {
      continue;
    }
    const before = kept.at(-2);
    if (before && last && ((before.x === last.x && last.x === point.x) || (before.y === last.y && last.y === point.y))) {
      kept[kept.length - 1] = point;
    } else {
      kept.push(point);
    }
  }
  return kept;
}

/** The links and boxes as layOut takes them, checked: the boxes beside others, the stacks, and which links rank. */
interface Setup {
  axis: Axis;
  direction: Direction;
  spacing: Spacing;
  base: Map<string, Box>;
  links: LayeredLink[];
  stacks: Stack[];
  stackOf: Map<string, string>;
  besides: Map<string, string>;
  sideways: LayeredLink[];
  ranked: LayeredLink[];
  besideLines: Map<string, LayeredLink[]>;
  gaps: Map<string, number>;
}

/** The box dagre places for an end: its stack, else the box itself. */
function lift(setup: Setup, id: string): string {
  return setup.stackOf.get(id) ?? id;
}

/**
 * Checks the boxes and links: a box keeps its place beside another only if each of its links joins it to that box, and
 * only in a top-to-bottom drawing; no link joins two members of one stack.
 */
function setUp(boxes: Box[], links: LayeredLink[], direction: Direction, spacing: Spacing, options: { beside?: ReadonlyMap<string, string>; stacks?: Stack[] }): Setup {
  const base = new Map(boxes.map(box => [box.id, box]));
  const stacks = options.stacks ?? [];
  const stackOf = new Map(stacks.flatMap(stack => stack.members.map(member => [member.id, stack.id] as const)));
  const asked = options.beside ?? new Map<string, string>();
  const besides = new Map([...asked].filter(([leaf, anchor]) => (
    base.has(leaf) && base.has(anchor) && !asked.has(anchor) && !stackOf.has(leaf) && !stackOf.has(anchor)
    && links.every(link => (link.from !== leaf && link.to !== leaf) || link.from === anchor || link.to === anchor)
  )));
  if (besides.size > 0 && direction !== 'TB') {
    throw new Error('a box stands beside another only in a top-to-bottom drawing');
  }
  const sideways = links.filter(link => besides.get(link.from) === link.to || besides.get(link.to) === link.from);
  const ranked = links.filter(link => !sideways.includes(link));
  for (const link of ranked) {
    if (link.from !== link.to && (stackOf.get(link.from) ?? link.from) === (stackOf.get(link.to) ?? link.to)) {
      throw new Error(`the layered drawing joins ${link.from} and ${link.to}, two members of one stack`);
    }
  }
  const besideLines = Map.groupBy(sideways, link => (besides.has(link.from) ? link.to : link.from));
  const gaps = new Map([...besideLines].map(([anchor, lines]) => [anchor, Math.max(spacing.node, channelRoom(lines.length))]));
  return { axis: AXES[direction], direction, spacing, base, links, stacks, stackOf, besides, sideways, ranked, besideLines, gaps };
}

/** The boxes dagre places at some sizes: each box outside a stack, and each stack with its members one after another. */
interface Sizes {
  boxes: Box[];
  members: Map<string, Rect>;
  ranked: string[];
}

function sized(setup: Setup, sizes: ReadonlyMap<string, Box>): Sizes {
  const { axis, stacks, stackOf, besides } = setup;
  const members = new Map<string, Rect>();
  const outside = [...sizes.values()].filter(box => !stackOf.has(box.id));
  const stacked = stacks.map((stack): Box => {
    let cross = 0;
    let main = 0;
    for (const member of stack.members) {
      const box = sizes.get(member.id);
      if (!box) {
        throw new Error(`the stack ${stack.id} holds ${member.id}, which is no box`);
      }
      members.set(member.id, axis.rect(member.indent, cross, axis.mainSize(box), axis.crossSize(box)));
      cross += axis.crossSize(box) + STACK_GAP;
      main = Math.max(main, member.indent + axis.mainSize(box));
    }
    return { id: stack.id, ...axis.rect(0, 0, main, Math.max(0, cross - STACK_GAP)) };
  });
  const all = [...outside, ...stacked];
  return { boxes: all.map(box => ({ id: box.id, width: box.width, height: box.height })), members, ranked: all.map(box => box.id).filter(id => !besides.has(id)) };
}

/** Places sized boxes with dagre, by layout.ts's arrange, and ranks them. */
function placeBoxes(setup: Setup, sizes: Sizes, dagreLinks: Link[]): { arrangement: Arrangement; ranks: Map<string, number> } {
  const arrangement = arrange(sizes.boxes, dagreLinks, setup.direction, setup.spacing, setup.besides, setup.gaps);
  return { arrangement, ranks: ranksOf(arrangement, sizes.ranked) };
}

/**
 * The links dagre ranks by, between the boxes it places: each ranked link but a box's link to itself, turned round when
 * `reversed`. A link left out of the ranking (an undeclared dependency against the design) runs the way the other links
 * rank its ends.
 */
function dagreLinksOf(setup: Setup, sizes: Sizes): Link[] {
  const ranking = (forward: (link: LayeredLink) => boolean, keepUnranked: boolean): Link[] => setup.ranked.filter(link => link.from !== link.to).map(link => {
    const [from, to] = forward(link) ? [lift(setup, link.from), lift(setup, link.to)] : [lift(setup, link.to), lift(setup, link.from)];
    return { id: link.id, from, to, unranked: keepUnranked && link.unranked === true };
  });
  if (!setup.ranked.some(link => link.unranked)) {
    return ranking(link => !link.reversed, false);
  }
  const { ranks } = placeBoxes(setup, sizes, ranking(link => !link.reversed, true));
  return ranking(link => (link.unranked ? rankIn(ranks, lift(setup, link.from)) <= rankIn(ranks, lift(setup, link.to)) : !link.reversed), false);
}

/** The sides each link meets its two boxes on, by the ranks. */
function sidesOf(setup: Setup, ranks: ReadonlyMap<string, number>): Map<string, [Side, Side]> {
  return new Map<string, [Side, Side]>(setup.links.map(link => {
    if (setup.sideways.includes(link)) {
      return [link.id, ['beside', 'beside']];
    }
    const from = rankIn(ranks, lift(setup, link.from));
    const to = rankIn(ranks, lift(setup, link.to));
    if (link.from === link.to) {
      return [link.id, ['end', 'end']];
    }
    if (from === to) {
      throw new Error(`the layered drawing ranks ${link.from} and ${link.to}, which a line joins, alike`);
    }
    return [link.id, from < to ? ['end', 'start'] : ['start', 'end']];
  }));
}

/** The boxes grown along each side that meets more lines than the side holds. */
function grownBoxes(setup: Setup, sides: ReadonlyMap<string, [Side, Side]>): Map<string, Box> {
  const { axis } = setup;
  const lineCounts = new Map<string, number>();
  for (const link of setup.links) {
    const [fromSide, toSide] = sides.get(link.id) ?? ['end', 'start'];
    for (const key of [`${link.from} ${fromSide}`, `${link.to} ${toSide}`]) {
      lineCounts.set(key, (lineCounts.get(key) ?? 0) + 1);
    }
  }
  return new Map([...setup.base].map(([id, box]) => {
    const across = Math.max(lineCounts.get(`${id} start`) ?? 0, lineCounts.get(`${id} end`) ?? 0);
    const along = lineCounts.get(`${id} beside`) ?? 0;
    const size = axis.rect(0, 0, Math.max(axis.mainSize(box), along > 0 ? portRoom(along) : 0), Math.max(axis.crossSize(box), across > 0 ? portRoom(across) : 0));
    return [id, { id, width: size.width, height: size.height }];
  }));
}

/** The boxes in their ranks: where each stands, a stack's members inside it, and each rank's band along the main axis. */
interface Placed {
  rects: Map<string, Rect>;
  rankOf: (id: string) => number;
  bands: { start: number; end: number; centre: number }[];
  bends: Arrangement['bends'];
}

function rectOf(placed: Placed, id: string): Rect {
  const rect = placed.rects.get(id);
  if (!rect) {
    throw new Error(`the layered drawing placed no box for ${id}`);
  }
  return rect;
}

function placedOf(setup: Setup, sizes: Sizes, arrangement: Arrangement, ranks: Map<string, number>): Placed {
  const { axis } = setup;
  const rects = new Map(arrangement.rects);
  for (const [id, rect] of sizes.members) {
    const at = rects.get(lift(setup, id));
    if (at) {
      rects.set(id, { ...rect, x: rect.x + at.x, y: rect.y + at.y });
    }
  }
  const rankOf = (id: string) => rankIn(ranks, lift(setup, setup.besides.get(id) ?? id));
  const bands = Array.from({ length: Math.max(0, ...ranks.values()) + 1 }, () => ({ start: Infinity, end: -Infinity, centre: 0 }));
  for (const box of sizes.boxes) {
    const rect = rects.get(box.id);
    const band = bands[rankOf(box.id)];
    if (rect && band) {
      band.start = Math.min(band.start, axis.mainStart(rect));
      band.end = Math.max(band.end, axis.mainStart(rect) + axis.mainSize(rect));
      band.centre = arrangement.centres.get(box.id) ?? band.centre;
    }
  }
  return { rects, rankOf, bands, bends: arrangement.bends };
}

/** The lines' stops rank by rank, and every place a line meets a box. */
interface Stops {
  stops: Map<string, Stop[]>;
  meets: Meet[];
  besideMeets: Map<string, readonly [Meet, Meet]>;
}

/** Each ranked line's stops: its first box's port, where it passes the ranks between, its last box's port. */
function stopsOf(setup: Setup, placed: Placed, sides: ReadonlyMap<string, [Side, Side]>): Stops {
  const { axis, besides } = setup;
  const meets: Meet[] = [];
  const meet = (key: string, box: string, side: Side, toward: number): Meet => {
    const item: Meet = { key, box, side, wall: side === 'start' || (side === 'beside' && besides.has(box)) ? 'end' : 'start', toward };
    meets.push(item);
    return item;
  };
  const crossCentre = (id: string) => axis.crossStart(rectOf(placed, id)) + axis.crossSize(rectOf(placed, id)) / 2;
  const mainCentre = (id: string) => axis.mainStart(rectOf(placed, id)) + axis.mainSize(rectOf(placed, id)) / 2;
  const stops = new Map<string, Stop[]>();
  for (const link of setup.ranked) {
    const [fromSide, toSide] = sides.get(link.id) ?? ['end', 'start'];
    const from = placed.rankOf(link.from);
    const to = placed.rankOf(link.to);
    const step = Math.sign(to - from);
    const passes: Stop[] = [];
    for (let rank = from + step; rank !== to; rank += step) {
      const centre = placed.bands[rank]?.centre ?? 0;
      const point = (placed.bends.get(link.id) ?? []).find(item => Math.abs(axis.main(item) - centre) < 1);
      if (!point) {
        throw new Error(`dagre left ${link.id} no room in rank ${rank}`);
      }
      passes.push({ rank, pass: axis.cross(point) });
    }
    const first = passes[0];
    const last = passes.at(-1);
    const own = link.from === link.to;
    stops.set(link.id, [
      { rank: from, meet: meet(`${link.id} from`, link.from, fromSide, own ? -Infinity : first && 'pass' in first ? first.pass : crossCentre(link.to)) },
      ...passes,
      { rank: to, meet: meet(`${link.id} to`, link.to, toSide, own ? Infinity : last && 'pass' in last ? last.pass : crossCentre(link.from)) },
    ]);
  }
  const besideMeets = new Map(setup.sideways.map(link => [link.id, [meet(`${link.id} from`, link.from, 'beside', mainCentre(link.to)), meet(`${link.id} to`, link.to, 'beside', mainCentre(link.from))] as const]));
  return { stops, meets, besideMeets };
}

/** The ports: each side's lines in the order of where they head, spread over the side's ports of its wall's parity. */
function portsOf(axis: Axis, placed: Placed, meets: Meet[]): (item: Meet) => number {
  const ports = new Map<string, number>();
  for (const group of Map.groupBy(meets, item => `${item.box} ${item.side}`).values()) {
    const sorted = group.sort((a, b) => a.toward - b.toward || (a.key < b.key ? -1 : 1));
    for (const [index, item] of sorted.entries()) {
      const rect = rectOf(placed, item.box);
      const [from, length] = item.side === 'beside' ? [axis.mainStart(rect), axis.mainSize(rect)] : [axis.crossStart(rect), axis.crossSize(rect)];
      ports.set(item.key, portFor(index, sorted.length, portsAlong(from, from + length, item.wall)));
    }
  }
  return item => {
    const port = ports.get(item.key);
    if (port === undefined) {
      throw new Error(`the layered drawing gave ${item.key} no port`);
    }
    return port;
  };
}

function legKey(link: string, index: number): string {
  return `${link} ${index}`;
}

/**
 * The legs of each channel: the one after each rank holds the legs between it and the next, and the lines from a box
 * back to it. A marked line has its mark on its first leg beside the box it leaves, or on its last beside the box it
 * enters, whichever side meets fewer lines; `marked` names the leg.
 */
function channelsOf(setup: Setup, placed: Placed, found: Stops, portOf: (item: Meet) => number): { channels: ChannelLine[][]; marked: Map<string, number> } {
  const channels = Array.from({ length: placed.bands.length }, () => [] as ChannelLine[]);
  const crossAt = (stop: Stop, wall: Wall) => ('meet' in stop ? portOf(stop.meet) : portNear(stop.pass, wall));
  const sideCounts = Map.groupBy(found.meets, item => `${item.box} ${item.side}`);
  const linesAt = (stop: Stop | undefined) => (stop && 'meet' in stop ? sideCounts.get(`${stop.meet.box} ${stop.meet.side}`)?.length ?? 0 : Infinity);
  const marked = new Map<string, number>();
  for (const link of setup.ranked) {
    const stops = found.stops.get(link.id) ?? [];
    const onFirst = link.mark !== undefined && linesAt(stops[0]) < linesAt(stops.at(-1));
    for (const [index, stop] of stops.slice(0, -1).entries()) {
      const next = stops[index + 1];
      if (next === undefined) {
        continue;
      }
      // A leg runs from the start wall to the end wall going down the ranks, the other way going up, and from the start
      // wall back to it for a box's line to itself.
      const [leave, arrive]: [Wall, Wall] = next.rank > stop.rank ? ['start', 'end'] : next.rank < stop.rank ? ['end', 'start'] : ['start', 'start'];
      const markHere = link.mark !== undefined && (onFirst ? index === 0 : index === stops.length - 2);
      if (markHere) {
        marked.set(link.id, index);
      }
      channels[Math.min(stop.rank, next.rank)]?.push({
        id: legKey(link.id, index), from: { y: crossAt(stop, leave), wall: leave }, to: { y: crossAt(next, arrive), wall: arrive },
        ...(link.mark && markHere ? { mark: { wall: onFirst ? leave : arrive, ...link.mark } } : {}),
      });
    }
  }
  return { channels, marked };
}

/**
 * Plans each channel and moves each rank along the main axis so that the channel before it is as wide as its tracks
 * need; the boxes move with their ranks. The legs' routes and the ranks' moved bands.
 */
function widened(setup: Setup, placed: Placed, channels: ChannelLine[][]): { legs: Map<string, Route>; bands: { start: number; end: number }[] } {
  const { axis, spacing } = setup;
  const last = placed.bands.length - 1;
  const plans = channels.map((lines, rank) => (rank < last || lines.length > 0 ? planChannel(lines, rank < last ? spacing.rank : 0) : undefined));
  const shifts: number[] = [];
  let at = placed.bands[0]?.start ?? 0;
  for (const [rank, band] of placed.bands.entries()) {
    shifts.push(at - band.start);
    at += band.end - band.start + (plans[rank]?.width ?? 0);
  }
  for (const [id, rect] of placed.rects) {
    const shift = shifts[placed.rankOf(id)] ?? 0;
    placed.rects.set(id, axis.rect(axis.mainStart(rect) + shift, axis.crossStart(rect), axis.mainSize(rect), axis.crossSize(rect)));
  }
  const bands = placed.bands.map((band, rank) => ({ start: band.start + (shifts[rank] ?? 0), end: band.end + (shifts[rank] ?? 0) }));
  const legs = new Map(channels.flatMap((lines, rank) => {
    const plan = plans[rank];
    const band = bands[rank];
    return plan && band ? [...routeChannel(lines, plan, band.end)] : [];
  }));
  return { legs, bands };
}

/** The ranked lines' points: from the first box's side to its rank's edge, through each channel and rank passed, into the last box. */
function rankedPoints(setup: Setup, placed: Placed, stops: Map<string, Stop[]>, portOf: (item: Meet) => number, legs: Map<string, Route>, bands: { start: number; end: number }[]): Map<string, Point[]> {
  const { axis } = setup;
  const points = new Map<string, Point[]>();
  for (const [id, line] of stops) {
    const path: Point[] = [];
    for (const [index, stop] of line.entries()) {
      const band = bands[stop.rank];
      if (!band) {
        throw new Error(`the layered drawing has no rank ${stop.rank}`);
      }
      if ('meet' in stop) {
        const rect = rectOf(placed, stop.meet.box);
        const port = portOf(stop.meet);
        const side = stop.meet.side === 'start' ? [axis.mainStart(rect), band.start] : [axis.mainStart(rect) + axis.mainSize(rect), band.end];
        const ends = side.map(main => axis.point(main, port));
        path.push(...(index === 0 ? ends : ends.reverse()));
      } else {
        // A pass enters its rank at one wall's port and leaves at the other's, stepping over in the middle.
        const down = stop.rank > (line[index - 1]?.rank ?? stop.rank);
        const [enter, leave]: [Wall, Wall] = down ? ['end', 'start'] : ['start', 'end'];
        const [first, last] = down ? [band.start, band.end] : [band.end, band.start];
        const middle = (band.start + band.end) / 2;
        path.push(axis.point(first, portNear(stop.pass, enter)), axis.point(middle, portNear(stop.pass, enter)), axis.point(middle, portNear(stop.pass, leave)), axis.point(last, portNear(stop.pass, leave)));
      }
      const leg = legs.get(legKey(id, index));
      if (leg) {
        path.push(...leg.points.map(point => axis.point(point.x, point.y)));
      }
    }
    points.set(id, path);
  }
  return points;
}

/** The lines to the boxes beside others, each through the channel between its two boxes. */
function besidePoints(setup: Setup, placed: Placed, found: Stops, portOf: (item: Meet) => number): Map<string, Point[]> {
  const points = new Map<string, Point[]>();
  for (const [anchor, lines] of setup.besideLines) {
    const rect = rectOf(placed, anchor);
    const channel = lines.map((link): ChannelLine => {
      const [from, to] = found.besideMeets.get(link.id) ?? [];
      if (!from || !to) {
        throw new Error(`the layered drawing met ${link.id} at no box`);
      }
      return { id: link.id, from: { y: portOf(from), wall: from.wall }, to: { y: portOf(to), wall: to.wall } };
    });
    for (const [id, route] of routeChannel(channel, planChannel(channel, setup.gaps.get(anchor) ?? setup.spacing.node), rect.x + rect.width)) {
      points.set(id, route.points);
    }
  }
  return points;
}

/**
 * The drawing moved to start at the origin, by whole steps of the port grid so that every port keeps its parity; a
 * marked line's label point is its mark's centre.
 */
function atOrigin(rects: Map<string, Rect>, points: Map<string, Point[]>, mids: Map<string, Point>): Layered {
  const corners = [...rects.values()].flatMap(rect => [{ x: rect.x, y: rect.y }, { x: rect.x + rect.width, y: rect.y + rect.height }]).concat([...points.values()].flat());
  const left = Math.floor(Math.min(0, ...corners.map(point => point.x)) / PORT_GRID) * PORT_GRID;
  const top = Math.floor(Math.min(0, ...corners.map(point => point.y)) / PORT_GRID) * PORT_GRID;
  return {
    rects: new Map([...rects].map(([id, rect]) => [id, { ...rect, x: rect.x - left, y: rect.y - top }])),
    routes: new Map([...points].map(([id, path]) => {
      const moved = tidy(path.map(point => ({ x: point.x - left, y: point.y - top })));
      const mid = mids.get(id);
      return [id, mid ? routeThrough(moved, { x: mid.x - left, y: mid.y - top }) : routeThrough(moved)];
    })),
    width: Math.max(0, ...corners.map(point => point.x - left)),
    height: Math.max(0, ...corners.map(point => point.y - top)),
  };
}

/**
 * Places the boxes in ranks and routes every link. `beside` maps a box to the box it stands beside (TB only), as
 * arrange's does; a box keeps that place only if each of its links joins it to that box. `stacks` names boxes that
 * stand as one; no link joins two members of one stack. The boxes are ranked once at their own sizes, grown where a
 * side meets more lines than it holds, and placed again; then the lines are routed.
 */
export function layOut(boxes: Box[], links: LayeredLink[], direction: Direction, spacing: Spacing, options: { beside?: ReadonlyMap<string, string>; stacks?: Stack[] } = {}): Layered {
  const setup = setUp(boxes, links, direction, spacing, options);
  const baseSizes = sized(setup, setup.base);
  const dagreLinks = dagreLinksOf(setup, baseSizes);
  const sizes = sized(setup, grownBoxes(setup, sidesOf(setup, placeBoxes(setup, baseSizes, dagreLinks).ranks)));
  const { arrangement, ranks } = placeBoxes(setup, sizes, dagreLinks);
  const placed = placedOf(setup, sizes, arrangement, ranks);
  const found = stopsOf(setup, placed, sidesOf(setup, ranks));
  const portOf = portsOf(setup.axis, placed, found.meets);
  const { channels, marked } = channelsOf(setup, placed, found, portOf);
  const { legs, bands } = widened(setup, placed, channels);
  const points = new Map([...rankedPoints(setup, placed, found.stops, portOf, legs, bands), ...besidePoints(setup, placed, found, portOf)]);
  const mids = new Map([...marked].flatMap(([id, index]) => {
    const leg = legs.get(legKey(id, index));
    return leg ? [[id, setup.axis.point(leg.mid.x, leg.mid.y)] as const] : [];
  }));
  return atOrigin(placed.rects, points, mids);
}
