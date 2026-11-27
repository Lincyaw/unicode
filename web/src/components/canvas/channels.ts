import { routeThrough, type Route } from './layout';

/*
 * Lines through a channel: the gap between two columns of boxes. A line leaves one wall of the channel and arrives at
 * the same wall or the other; it runs across to a track of its own, along the track, and across to where it arrives.
 * Two lines share a track only where their runs along it are far apart, so no line lies on another. A line that
 * carries a mark (a count) has it beside one wall, in a column of marks that the tracks keep clear of: the marks of
 * a wall stand in as many sub-columns as it takes for no two of them to overlap.
 */

/** A wall of a channel: the one at its start (left) or at its end (right). */
export type Wall = 'start' | 'end';

/** A line through a channel: the height and the wall where it leaves, where it arrives, and the mark it carries, if any. */
export interface ChannelLine {
  id: string;
  from: { y: number; wall: Wall };
  to: { y: number; wall: Wall };

  /** A mark to stand on the line beside a wall the line meets, at the line's height there, at this size */
  mark?: { wall: Wall; width: number; height: number };
}

/** Where the marks beside one wall stand: the column's width, and each mark's centre, from the column's near edge. */
interface MarkColumn {
  width: number;
  centres: Map<string, number>;
}

/** How the lines of a channel run: the channel's width, the track of each line that turns, and the marks beside each wall. */
export interface ChannelPlan {
  width: number;
  tracks: Map<string, number>;
  marks: Record<Wall, MarkColumn>;
}

/**
 * How far apart two tracks run: further than half a line's click target (canvas.module.css's .hit), so no line's click
 * target covers the next line, and each line keeps a stretch a click reaches.
 */
const TRACK_GAP = 8;

/** How far the outermost tracks keep from the walls, so an arrowhead has room. */
const WALL_ROOM = 24;

/** How far marks keep from a wall, from the tracks and from each other. */
const MARK_GAP = 8;

/** Whether a line runs straight across, from one wall to the other at one height. */
function straight(line: ChannelLine): boolean {
  return line.from.wall !== line.to.wall && Math.abs(line.from.y - line.to.y) < 0.5;
}

/** The height where a line meets a wall: where it leaves, else where it arrives. */
function heightAt(line: ChannelLine, wall: Wall): number {
  return line.from.wall === wall ? line.from.y : line.to.y;
}

/** The marks beside a wall, set in sub-columns: each mark in the first whose last mark ends a gap above it. */
function markColumn(lines: ChannelLine[], wall: Wall): MarkColumn {
  const marks = lines.flatMap(line => (line.mark?.wall === wall ? [{ id: line.id, y: heightAt(line, wall), ...line.mark }] : []))
    .sort((a, b) => a.y - b.y);
  const columns: { bottom: number; width: number; ids: string[] }[] = [];
  for (const mark of marks) {
    const top = mark.y - mark.height / 2;
    const column = columns.find(item => item.bottom + MARK_GAP <= top);
    if (column) {
      column.bottom = mark.y + mark.height / 2;
      column.width = Math.max(column.width, mark.width);
      column.ids.push(mark.id);
    } else {
      columns.push({ bottom: mark.y + mark.height / 2, width: mark.width, ids: [mark.id] });
    }
  }
  const centres = new Map<string, number>();
  let x = 0;
  for (const column of columns) {
    x += MARK_GAP;
    column.ids.forEach(id => centres.set(id, x + column.width / 2));
    x += column.width;
  }
  return { width: columns.length > 0 ? x + MARK_GAP : 0, centres };
}

/**
 * Gives each line that turns a track: the lines in order of where their runs start, each on the first track whose
 * last run ends a gap before it starts. The channel is as wide as its tracks and its marks need, and at least `min`.
 */
export function planChannel(lines: ChannelLine[], min: number): ChannelPlan {
  const runs = lines.filter(line => !straight(line))
    .map(line => ({ id: line.id, top: Math.min(line.from.y, line.to.y), bottom: Math.max(line.from.y, line.to.y) }))
    .sort((a, b) => a.top - b.top || a.bottom - b.bottom);
  const ends: number[] = [];
  const tracks = new Map<string, number>();
  for (const run of runs) {
    const free = ends.findIndex(end => end + TRACK_GAP <= run.top);
    const track = free >= 0 ? free : ends.length;
    ends[track] = run.bottom;
    tracks.set(run.id, track);
  }
  const marks = { start: markColumn(lines, 'start'), end: markColumn(lines, 'end') };
  const needed = marks.start.width + 2 * WALL_ROOM + Math.max(0, ends.length - 1) * TRACK_GAP + marks.end.width;
  return { width: Math.max(min, needed), tracks, marks };
}

/**
 * Routes the lines of a planned channel whose start wall stands at `start`. The tracks are centred between the mark
 * columns; a marked line's label point is its mark's centre, any other line's the middle of its run.
 */
export function routeChannel(lines: ChannelLine[], plan: ChannelPlan, start: number): Map<string, Route> {
  const count = Math.max(0, ...[...plan.tracks.values()].map(track => track + 1));
  const inner = plan.width - plan.marks.start.width - plan.marks.end.width;
  const first = start + plan.marks.start.width + (inner - Math.max(0, count - 1) * TRACK_GAP) / 2;
  const wallAt = (wall: Wall) => (wall === 'start' ? start : start + plan.width);
  const markAt = (line: ChannelLine) => {
    const wall = line.mark?.wall;
    const centre = wall && plan.marks[wall].centres.get(line.id);
    if (wall === undefined || centre === undefined) {
      return undefined;
    }
    return { x: wall === 'start' ? start + centre : start + plan.width - plan.marks.end.width + centre, y: heightAt(line, wall) };
  };
  return new Map(lines.map(line => {
    const from = { x: wallAt(line.from.wall), y: line.from.y };
    const to = { x: wallAt(line.to.wall), y: line.to.y };
    const track = plan.tracks.get(line.id);
    if (track === undefined) {
      return [line.id, routeThrough([from, to], markAt(line))];
    }
    const x = first + track * TRACK_GAP;
    return [line.id, routeThrough([from, { x, y: from.y }, { x, y: to.y }, to], markAt(line) ?? { x, y: (from.y + to.y) / 2 })];
  }));
}
