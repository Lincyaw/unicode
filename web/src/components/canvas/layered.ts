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

/** A link to draw from `from` to `to`; `reversed` ranks it the other way round, such as a call back into a view's input. */
export interface LayeredLink extends Link {
  reversed?: boolean;
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

/**
 * Places the boxes in ranks and routes every link. `beside` maps a box to the box it stands beside (TB only), as
 * arrange's does; a box keeps that place only if each of its links joins it to that box. `stacks` names boxes that
 * stand as one; no link joins two members of one stack.
 */
export function layOut(boxes: Box[], links: LayeredLink[], direction: Direction, spacing: Spacing, options: { beside?: ReadonlyMap<string, string>; stacks?: Stack[] } = {}): Layered {
  const axis = AXES[direction];
  const base = new Map(boxes.map(box => [box.id, box]));
  const stacks = options.stacks ?? [];
  const stackOf = new Map(stacks.flatMap(stack => stack.members.map(member => [member.id, stack.id] as const)));
  const lift = (id: string) => stackOf.get(id) ?? id;
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
    if (link.from !== link.to && lift(link.from) === lift(link.to)) {
      throw new Error(`the layered drawing joins ${link.from} and ${link.to}, two members of one stack`);
    }
  }
  const besideLines = Map.groupBy(sideways, link => (besides.has(link.from) ? link.to : link.from));
  const gaps = new Map([...besideLines].map(([anchor, lines]) => [anchor, Math.max(spacing.node, channelRoom(lines.length))]));

  // The boxes dagre places, at their sizes: each box outside a stack, and each stack with its members one after another.
  const placed = (sizes: ReadonlyMap<string, Box>) => {
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
  };
  const forDagre = (forward: (link: LayeredLink) => boolean, keepUnranked: boolean): Link[] => ranked.filter(link => link.from !== link.to).map(link => {
    const [from, to] = forward(link) ? [lift(link.from), lift(link.to)] : [lift(link.to), lift(link.from)];
    return { id: link.id, from, to, unranked: keepUnranked && link.unranked === true };
  });
  const place = (sizes: ReturnType<typeof placed>, dagreLinks: Link[]) => {
    const arrangement = arrange(sizes.boxes, dagreLinks, direction, spacing, besides, gaps);
    return { arrangement, ranks: ranksOf(arrangement, sizes.ranked) };
  };

  // A link left out of the ranking (an undeclared dependency against the design) runs the way the others rank its ends.
  const baseSizes = placed(base);
  const firstRanks = ranked.some(link => link.unranked) ? place(baseSizes, forDagre(link => !link.reversed, true)).ranks : undefined;
  const dagreLinks = forDagre(link => (firstRanks && link.unranked ? rankIn(firstRanks, lift(link.from)) <= rankIn(firstRanks, lift(link.to)) : !link.reversed), false);

  // The sides each line meets, by the ranks; a box with more lines on a side than the side holds grows along it.
  const sidesOf = (ranks: ReadonlyMap<string, number>) => new Map<string, [Side, Side]>(links.map(link => {
    if (sideways.includes(link)) {
      return [link.id, ['beside', 'beside']];
    }
    const from = rankIn(ranks, lift(link.from));
    const to = rankIn(ranks, lift(link.to));
    if (link.from === link.to) {
      return [link.id, ['end', 'end']];
    }
    if (from === to) {
      throw new Error(`the layered drawing ranks ${link.from} and ${link.to}, which a line joins, alike`);
    }
    return [link.id, from < to ? ['end', 'start'] : ['start', 'end']];
  }));
  const lineCounts = new Map<string, number>();
  const firstSides = sidesOf(place(baseSizes, dagreLinks).ranks);
  for (const link of links) {
    const [fromSide, toSide] = firstSides.get(link.id) ?? ['end', 'start'];
    for (const key of [`${link.from} ${fromSide}`, `${link.to} ${toSide}`]) {
      lineCounts.set(key, (lineCounts.get(key) ?? 0) + 1);
    }
  }
  const grown = new Map([...base].map(([id, box]) => {
    const across = Math.max(lineCounts.get(`${id} start`) ?? 0, lineCounts.get(`${id} end`) ?? 0);
    const along = lineCounts.get(`${id} beside`) ?? 0;
    const size = axis.rect(0, 0, Math.max(axis.mainSize(box), along > 0 ? portRoom(along) : 0), Math.max(axis.crossSize(box), across > 0 ? portRoom(across) : 0));
    return [id, { id, width: size.width, height: size.height }];
  }));
  const sizes = placed(grown);
  const { arrangement, ranks } = place(sizes, dagreLinks);
  const sides = sidesOf(ranks);
  const rankOf = (id: string) => rankIn(ranks, lift(besides.get(id) ?? id));

  // Every box where it stands, a stack's members inside it.
  const rects = new Map(arrangement.rects);
  for (const [id, rect] of sizes.members) {
    const at = rects.get(lift(id));
    if (at) {
      rects.set(id, { ...rect, x: rect.x + at.x, y: rect.y + at.y });
    }
  }
  const rectOf = (id: string) => {
    const rect = rects.get(id);
    if (!rect) {
      throw new Error(`the layered drawing placed no box for ${id}`);
    }
    return rect;
  };
  const rankCount = Math.max(0, ...ranks.values()) + 1;
  const bands = Array.from({ length: rankCount }, () => ({ start: Infinity, end: -Infinity, centre: 0 }));
  for (const box of sizes.boxes) {
    const rect = rectOf(box.id);
    const band = bands[rankOf(box.id)];
    if (band) {
      band.start = Math.min(band.start, axis.mainStart(rect));
      band.end = Math.max(band.end, axis.mainStart(rect) + axis.mainSize(rect));
      band.centre = arrangement.centres.get(box.id) ?? band.centre;
    }
  }
  const centreOf = (id: string) => axis.crossStart(rectOf(id)) + axis.crossSize(rectOf(id)) / 2;

  // Each ranked line's stops, rank by rank: its first box's port, where it passes the ranks between, its last box's port.
  const meets: Meet[] = [];
  const meet = (key: string, box: string, side: Side, toward: number): Meet => {
    const item: Meet = { key, box, side, wall: side === 'start' || (side === 'beside' && besides.has(box)) ? 'end' : 'start', toward };
    meets.push(item);
    return item;
  };
  const stopsOf = new Map<string, Stop[]>();
  for (const link of ranked) {
    const [fromSide, toSide] = sides.get(link.id) ?? ['end', 'start'];
    const from = rankOf(link.from);
    const to = rankOf(link.to);
    const step = Math.sign(to - from);
    const passes: Stop[] = [];
    for (let rank = from + step; rank !== to; rank += step) {
      const centre = bands[rank]?.centre ?? 0;
      const point = (arrangement.bends.get(link.id) ?? []).find(item => Math.abs(axis.main(item) - centre) < 1);
      if (!point) {
        throw new Error(`dagre left ${link.id} no room in rank ${rank}`);
      }
      passes.push({ rank, pass: axis.cross(point) });
    }
    const first = passes[0];
    const last = passes.at(-1);
    const own = link.from === link.to;
    stopsOf.set(link.id, [
      { rank: from, meet: meet(`${link.id} from`, link.from, fromSide, own ? -Infinity : first && 'pass' in first ? first.pass : centreOf(link.to)) },
      ...passes,
      { rank: to, meet: meet(`${link.id} to`, link.to, toSide, own ? Infinity : last && 'pass' in last ? last.pass : centreOf(link.from)) },
    ]);
  }
  const mainCentre = (id: string) => axis.mainStart(rectOf(id)) + axis.mainSize(rectOf(id)) / 2;
  const besideMeets = new Map(sideways.map(link => [link.id, [meet(`${link.id} from`, link.from, 'beside', mainCentre(link.to)), meet(`${link.id} to`, link.to, 'beside', mainCentre(link.from))]] as const));

  // The ports: each side's lines in the order of where they head, spread over the side's ports of its wall's parity.
  const ports = new Map<string, number>();
  for (const group of Map.groupBy(meets, item => `${item.box} ${item.side}`).values()) {
    const sorted = group.sort((a, b) => a.toward - b.toward || (a.key < b.key ? -1 : 1));
    for (const [index, item] of sorted.entries()) {
      const rect = rectOf(item.box);
      const [from, length] = item.side === 'beside' ? [axis.mainStart(rect), axis.mainSize(rect)] : [axis.crossStart(rect), axis.crossSize(rect)];
      ports.set(item.key, portFor(index, sorted.length, portsAlong(from, from + length, item.wall)));
    }
  }
  const portOf = (item: Meet) => {
    const port = ports.get(item.key);
    if (port === undefined) {
      throw new Error(`the layered drawing gave ${item.key} no port`);
    }
    return port;
  };
  /** Where a stop meets a wall of the channel beside its rank: its box's port, or the wall's port nearest its pass. */
  const crossAt = (stop: Stop, wall: Wall) => ('meet' in stop ? portOf(stop.meet) : portNear(stop.pass, wall));

  // The channels: the one after each rank holds the legs between it and the next, and the lines from a box back to it.
  const channels = Array.from({ length: rankCount }, () => [] as ChannelLine[]);
  const legKey = (link: string, index: number) => `${link} ${index}`;
  for (const [id, stops] of stopsOf) {
    for (const [index, stop] of stops.slice(0, -1).entries()) {
      const next = stops[index + 1];
      if (next === undefined) {
        continue;
      }
      // A leg runs from the start wall to the end wall going down the ranks, the other way going up, and from the start
      // wall back to it for a box's line to itself.
      const [leave, arrive]: [Wall, Wall] = next.rank > stop.rank ? ['start', 'end'] : next.rank < stop.rank ? ['end', 'start'] : ['start', 'start'];
      channels[Math.min(stop.rank, next.rank)]?.push({ id: legKey(id, index), from: { y: crossAt(stop, leave), wall: leave }, to: { y: crossAt(next, arrive), wall: arrive } });
    }
  }
  const plans = channels.map((lines, rank) => (rank < rankCount - 1 || lines.length > 0 ? planChannel(lines, rank < rankCount - 1 ? spacing.rank : 0) : undefined));

  // Each rank moves along the main axis so that the channel before it is as wide as its tracks need.
  const shifts: number[] = [];
  let at = bands[0]?.start ?? 0;
  for (const [rank, band] of bands.entries()) {
    shifts.push(at - band.start);
    at += band.end - band.start + (plans[rank]?.width ?? 0);
  }
  for (const [id, rect] of rects) {
    const shift = shifts[rankOf(id)] ?? 0;
    rects.set(id, axis.rect(axis.mainStart(rect) + shift, axis.crossStart(rect), axis.mainSize(rect), axis.crossSize(rect)));
  }
  const moved = bands.map((band, rank) => ({ start: band.start + (shifts[rank] ?? 0), end: band.end + (shifts[rank] ?? 0) }));
  const bandAt = (rank: number) => {
    const band = moved[rank];
    if (!band) {
      throw new Error(`the layered drawing has no rank ${rank}`);
    }
    return band;
  };
  const legs = new Map(channels.flatMap((lines, rank) => {
    const plan = plans[rank];
    return plan ? [...routeChannel(lines, plan, bandAt(rank).end)] : [];
  }));

  // The routes: from the first box's side to its rank's edge, through each channel and each rank passed, into the last box.
  const points = new Map<string, Point[]>();
  for (const [id, stops] of stopsOf) {
    const path: Point[] = [];
    for (const [index, stop] of stops.entries()) {
      const band = bandAt(stop.rank);
      if ('meet' in stop) {
        const rect = rectOf(stop.meet.box);
        const port = portOf(stop.meet);
        const side = stop.meet.side === 'start' ? [axis.mainStart(rect), band.start] : [axis.mainStart(rect) + axis.mainSize(rect), band.end];
        const ends = side.map(main => axis.point(main, port));
        path.push(...(index === 0 ? ends : ends.reverse()));
      } else {
        // A pass enters its rank at one wall's port and leaves at the other's, stepping over in the middle.
        const down = stop.rank > (stops[index - 1]?.rank ?? stop.rank);
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

  // The lines to the boxes beside others, each through the channel between its two boxes.
  for (const [anchor, lines] of besideLines) {
    const rect = rectOf(anchor);
    const channel = lines.map((link): ChannelLine => {
      const [from, to] = besideMeets.get(link.id) ?? [];
      if (!from || !to) {
        throw new Error(`the layered drawing met ${link.id} at no box`);
      }
      return { id: link.id, from: { y: portOf(from), wall: from.wall }, to: { y: portOf(to), wall: to.wall } };
    });
    for (const [id, route] of routeChannel(channel, planChannel(channel, gaps.get(anchor) ?? spacing.node), rect.x + rect.width)) {
      points.set(id, route.points);
    }
  }

  // The drawing starts at the origin, moved by whole steps of the port grid so that every port keeps its parity.
  const corners = [...rects.values()].flatMap(rect => [{ x: rect.x, y: rect.y }, { x: rect.x + rect.width, y: rect.y + rect.height }]).concat([...points.values()].flat());
  const left = Math.floor(Math.min(0, ...corners.map(point => point.x)) / PORT_GRID) * PORT_GRID;
  const top = Math.floor(Math.min(0, ...corners.map(point => point.y)) / PORT_GRID) * PORT_GRID;
  for (const [id, rect] of rects) {
    rects.set(id, { ...rect, x: rect.x - left, y: rect.y - top });
  }
  return {
    rects,
    routes: new Map([...points].map(([id, path]) => [id, routeThrough(tidy(path.map(point => ({ x: point.x - left, y: point.y - top }))))])),
    width: Math.max(0, ...corners.map(point => point.x - left)),
    height: Math.max(0, ...corners.map(point => point.y - top)),
  };
}
