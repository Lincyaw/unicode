import { routeThrough, type Route } from './layout';

/*
 * Lines through a channel: the gap between two columns of boxes. A line leaves one wall of the channel and arrives at
 * the same wall or the other; it runs across to a track of its own, along the track, and across to where it arrives.
 * Two lines share a track only where their runs along it are far apart, so no line lies on another.
 */

/** A wall of a channel: the one at its start (left) or at its end (right). */
export type Wall = 'start' | 'end';

/** A line through a channel: the height and the wall where it leaves, and where it arrives. */
export interface ChannelLine {
  id: string;
  from: { y: number; wall: Wall };
  to: { y: number; wall: Wall };
}

/** How the lines of a channel run: the channel's width, and the track each line that turns runs along. */
export interface ChannelPlan {
  width: number;
  tracks: Map<string, number>;
}

/**
 * How far apart two tracks run: further than half a line's click target (canvas.module.css's .hit), so no line's click
 * target covers the next line, and each line keeps a stretch a click reaches.
 */
const TRACK_GAP = 8;

/** How far the outermost tracks keep from the walls, so an arrowhead has room. */
const WALL_ROOM = 24;

/** Whether a line runs straight across, from one wall to the other at one height. */
function straight(line: ChannelLine): boolean {
  return line.from.wall !== line.to.wall && Math.abs(line.from.y - line.to.y) < 0.5;
}

/**
 * Gives each line that turns a track: the lines in order of where their runs start, each on the first track whose
 * last run ends a gap before it starts. The channel is as wide as its tracks need, and at least `min`.
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
  return { width: Math.max(min, 2 * WALL_ROOM + Math.max(0, ends.length - 1) * TRACK_GAP), tracks };
}

/** Routes the lines of a planned channel whose start wall stands at `start`; the tracks are centred in the channel. */
export function routeChannel(lines: ChannelLine[], plan: ChannelPlan, start: number): Map<string, Route> {
  const count = Math.max(0, ...[...plan.tracks.values()].map(track => track + 1));
  const first = start + (plan.width - Math.max(0, count - 1) * TRACK_GAP) / 2;
  const wallAt = (wall: Wall) => (wall === 'start' ? start : start + plan.width);
  return new Map(lines.map(line => {
    const from = { x: wallAt(line.from.wall), y: line.from.y };
    const to = { x: wallAt(line.to.wall), y: line.to.y };
    const track = plan.tracks.get(line.id);
    if (track === undefined) {
      return [line.id, routeThrough([from, to])];
    }
    const x = first + track * TRACK_GAP;
    return [line.id, routeThrough([from, { x, y: from.y }, { x, y: to.y }, to], { x, y: (from.y + to.y) / 2 })];
  }));
}
