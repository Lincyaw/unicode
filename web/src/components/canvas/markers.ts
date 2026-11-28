import type { Point, Rect, Route } from './layout';

/*
 * Where the labels and badges of a drawing's lines stand. Each is tried at its line's own label point, then at points
 * further along the line, on the sides it accepts, and takes the first place where it covers no box and no label or
 * badge placed before it.
 */

/** Where a label or badge stands against its line: on it, or beside it. */
export type Side = 'center' | 'right' | 'left' | 'above' | 'below';

/** A label stands beside its line; a badge stands on it, or beside it where the line has no free stretch. */
export const LABEL_SIDES: readonly Side[] = ['right', 'left', 'above', 'below'];
export const BADGE_SIDES: readonly Side[] = ['center', 'right', 'left', 'above', 'below'];

/** A label or badge to place on or beside its line, at the size it is drawn. */
export interface Spot {
  id: string;
  route: Route;
  width: number;
  height: number;

  /** The sides it accepts, the preferred first; beside a line across, right and left; beside a line down, above and below */
  sides: readonly Side[];
}

/** How far a label beside a line keeps from it, and how much room it keeps around itself. */
const GAP = 6;
const CLEAR = 2;

/** Where along its line a label is tried after the line's own label point: outward from the middle. */
const FRACTIONS = [0.5, 0.4, 0.6, 0.3, 0.7, 0.2, 0.8, 0.12, 0.88];

/** A point along the line at a fraction of its length, and whether the stretch it lies on runs down. */
function pointAlong(points: Point[], fraction: number): { point: Point; down: boolean } {
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

/** Whether a stretch of the line runs down at the point: the stretch the point lies on. */
function runsDown(points: Point[], at: Point): boolean {
  return points.slice(1).some((end, index) => {
    const start = points[index];
    return start !== undefined && start.x === end.x && Math.abs(at.x - start.x) < 0.5 && at.y >= Math.min(start.y, end.y) && at.y <= Math.max(start.y, end.y);
  });
}

/** The box a label of a size takes on a side of a point. */
function boxAt(point: Point, side: Side, width: number, height: number): Rect {
  switch (side) {
    case 'center':
      return { x: point.x - width / 2, y: point.y - height / 2, width, height };
    case 'right':
      return { x: point.x + GAP, y: point.y - height / 2, width, height };
    case 'left':
      return { x: point.x - GAP - width, y: point.y - height / 2, width, height };
    case 'above':
      return { x: point.x - width / 2, y: point.y - GAP - height, width, height };
    case 'below':
      return { x: point.x - width / 2, y: point.y + GAP, width, height };
  }
}

/** Whether a side suits a stretch: beside a line down is right or left of it, beside a line across above or below it. */
function suits(side: Side, down: boolean): boolean {
  return side === 'center' || (down ? side === 'right' || side === 'left' : side === 'above' || side === 'below');
}

function meets(a: Rect, b: Rect): boolean {
  return a.x - CLEAR < b.x + b.width && b.x < a.x + a.width + CLEAR && a.y - CLEAR < b.y + b.height && b.y < a.y + a.height + CLEAR;
}

/**
 * Places the spots in order, each where it covers none of the obstacles and none of the spots placed before it; a spot
 * with no such place takes its first choice.
 */
export function placeSpots(spots: Spot[], obstacles: Rect[]): Map<string, Rect> {
  const placed = new Map<string, Rect>();
  for (const spot of spots) {
    const tries = [{ point: spot.route.mid, down: runsDown(spot.route.points, spot.route.mid) }, ...FRACTIONS.map(fraction => pointAlong(spot.route.points, fraction))];
    const boxes = tries.flatMap(({ point, down }) => spot.sides.filter(side => suits(side, down)).map(side => boxAt(point, side, spot.width, spot.height)));
    const taken = [...obstacles, ...placed.values()];
    const free = boxes.find(box => !taken.some(other => meets(box, other)));
    placed.set(spot.id, free ?? boxes[0] ?? boxAt(spot.route.mid, 'center', spot.width, spot.height));
  }
  return placed;
}
