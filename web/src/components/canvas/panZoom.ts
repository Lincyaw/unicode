import { select } from 'd3-selection';
import 'd3-transition';
import { zoom, zoomIdentity, zoomTransform, type D3ZoomEvent, type ZoomBehavior, type ZoomTransform } from 'd3-zoom';
import { createContext, useContext, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react';
import { REDUCED_MOTION_QUERY } from '../../ui/media';

/*
 * How a canvas moves under the reader's hands, by d3-zoom: a pinch on the trackpad or the touch screen, or the wheel
 * with Ctrl or Cmd, zooms around the pointer; the plain wheel and two fingers on the trackpad pan; a drag pans, with
 * the mouse or one finger, and a drag that moves further than a click would does not click the node it started on.
 * The canvas never scrolls: what the keyboard focuses, and the node the reader asks for, are panned into view. Where
 * the reader left each drawing is kept for the session.
 */

/** The zoom's bounds. */
export const MIN_SCALE = 0.25;
export const MAX_SCALE = 2;

/** How far a press may move and still be a click, in pixels. */
const CLICK_DISTANCE = 4;

/** How much of the drawing, in pixels, stays in the canvas however far it is panned. */
const KEEP_IN_SIGHT = 48;

/** The pixels one line of a wheel that scrolls by lines stands for. */
const WHEEL_LINE = 16;

/**
 * How far one wheel event zooms, as a power of two: a pinch sends many small deltas and follows the fingers, while a
 * wheel's notch, a hundred pixels or a few lines, zooms by one step of about a fifth.
 */
const ZOOM_PER_PIXEL = 0.01;
const ZOOM_PER_LINE = 0.05;
const ZOOM_STEP = 0.25;

function wheelZoom(event: WheelEvent): number {
  const delta = -event.deltaY * (event.deltaMode === WheelEvent.DOM_DELTA_LINE ? ZOOM_PER_LINE : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? 1 : ZOOM_PER_PIXEL);
  return Math.max(-ZOOM_STEP, Math.min(ZOOM_STEP, delta));
}

/** How long a pan that brings something into view takes, in milliseconds; none when the reader asks for less motion. */
const REVEAL_DURATION = 300;

/** How close to the canvas's edge a box may sit before the canvas brings it into view. */
const EDGE_ROOM = 24;

/** The size of what a canvas draws, its margins included. */
export interface DrawingSize {
  width: number;
  height: number;
}

/** Names the drawing a canvas shows, such as a view with its input, so that the reader's zoom and pan are kept for it. */
export const DrawingKey = createContext<string | undefined>(undefined);

/** Asks the canvas around a box to bring a node's box into view once the drawing has redrawn, such as a card just unfolded. */
const RevealNode = createContext<((id: string) => void) | null>(null);
export const RevealProvider = RevealNode.Provider;

export function useReveal(): (id: string) => void {
  const reveal = useContext(RevealNode);
  if (reveal === null) {
    throw new Error('A box asks to be brought into view only on a canvas, which provides RevealNode');
  }
  return reveal;
}

const STORE_PREFIX = 'unicode.canvas.';

function isTransform(value: unknown): value is { k: number; x: number; y: number } {
  return typeof value === 'object' && value !== null && ['k', 'x', 'y'].every(key => typeof (value as Record<string, unknown>)[key] === 'number');
}

/** Where the reader left a drawing this session; none before the reader has moved it. */
function kept(key: string | undefined): ZoomTransform | undefined {
  const text = key === undefined ? null : sessionStorage.getItem(STORE_PREFIX + key);
  if (text === null) {
    return undefined;
  }
  const value: unknown = JSON.parse(text);
  if (!isTransform(value)) {
    throw new Error(`The session keeps the zoom and pan of the drawing ${key ?? ''} as {k, x, y} numbers, not ${text}`);
  }
  return zoomIdentity.translate(value.x, value.y).scale(value.k);
}

function keep(key: string | undefined, transform: ZoomTransform) {
  if (key !== undefined) {
    sessionStorage.setItem(STORE_PREFIX + key, JSON.stringify({ k: transform.k, x: transform.x, y: transform.y }));
  }
}

/**
 * Whether an event starts a gesture: the wheel only with Ctrl (a trackpad's pinch arrives so) or Cmd, as the plain
 * wheel pans below; a press with the main button or a touch anywhere but on a control lifted inside a box (a
 * reference, a link, a hint, a chevron), which keeps its own click. A press on a box's cover pans when it moves.
 */
function startsGesture(event: Event): boolean {
  if (event instanceof WheelEvent) {
    return event.ctrlKey || event.metaKey;
  }
  const pressed = event as MouseEvent;
  const lifted = event.target instanceof Element && event.target.closest('a, [data-ref], [data-lifted]') !== null;
  return !pressed.button && !pressed.ctrlKey && !lifted;
}

/** Holds a transform so that at least KEEP_IN_SIGHT pixels of the drawing stay inside the canvas. */
function keepInSight(transform: ZoomTransform, extent: [[number, number], [number, number]], size: DrawingSize): ZoomTransform {
  const [[left, top], [right, bottom]] = extent;
  const width = size.width * transform.k;
  const height = size.height * transform.k;
  const clamp = (value: number, low: number, high: number) => Math.min(Math.max(value, low), high);
  const x = clamp(transform.x, left + Math.min(KEEP_IN_SIGHT, width) - width, right - Math.min(KEEP_IN_SIGHT, width));
  const y = clamp(transform.y, top + Math.min(KEEP_IN_SIGHT, height) - height, bottom - Math.min(KEEP_IN_SIGHT, height));
  return x === transform.x && y === transform.y ? transform : zoomIdentity.translate(x, y).scale(transform.k);
}

/** The pan along one axis that brings a box into the room: to the middle when it fits, else to its start; none when it is in sight. */
function panAlong(start: number, size: number, roomStart: number, roomSize: number): number {
  const inSight = start >= roomStart + EDGE_ROOM && start + size <= roomStart + roomSize - EDGE_ROOM;
  if (inSight) {
    return 0;
  }
  return size <= roomSize - 2 * EDGE_ROOM ? roomStart + roomSize / 2 - (start + size / 2) : roomStart + EDGE_ROOM - start;
}

/** What a canvas can do with its drawing besides the reader's gestures. */
export interface PanZoom {
  /** The drawing's transform; null until the canvas has opened it */
  transform: ZoomTransform | null;

  /** Zooms to a scale around the middle of the canvas */
  zoomTo: (scale: number) => void;

  /** Zooms to show the whole drawing, no larger than its own size, in the middle of the canvas */
  fit: () => void;

  /** Pans an element of the drawing into view when it is out of sight, smoothly unless the reader asks for less motion */
  reveal: (element: Element) => void;
}

/**
 * The pan and zoom of a canvas's viewport. It opens at the transform the reader left the drawing at this session, else
 * at `opening`, which takes the viewport's width and height.
 */
export function usePanZoom(viewport: RefObject<HTMLDivElement | null>, size: DrawingSize, opening: (width: number, height: number) => ZoomTransform): PanZoom {
  const memory = useContext(DrawingKey);
  const [transform, setTransform] = useState<ZoomTransform | null>(null);
  const behavior = useRef<ZoomBehavior<HTMLDivElement, unknown> | null>(null);
  // The size and the opening are read when the canvas opens and when the reader acts, so they are kept current here.
  const latestSize = useRef(size);
  const latestOpening = useRef(opening);
  useLayoutEffect(() => {
    latestSize.current = size;
    latestOpening.current = opening;
  });

  const actions = useMemo(() => {
    const run = (act: (zooming: ZoomBehavior<HTMLDivElement, unknown>, element: HTMLDivElement) => void) => {
      const element = viewport.current;
      const zooming = behavior.current;
      if (!element || !zooming) {
        throw new Error('A canvas is moved only once it has opened its drawing');
      }
      act(zooming, element);
    };
    return {
      zoomTo: (scale: number) => run((zooming, element) => zooming.scaleTo(select(element), scale)),
      fit: () => run((zooming, element) => {
        const { width, height } = latestSize.current;
        const k = Math.max(MIN_SCALE, Math.min(1, element.clientWidth / width, element.clientHeight / height));
        zooming.transform(select(element), zoomIdentity.translate((element.clientWidth - width * k) / 2, (element.clientHeight - height * k) / 2).scale(k));
      }),
      reveal: (target: Element) => run((zooming, element) => {
        const box = target.getBoundingClientRect();
        const room = element.getBoundingClientRect();
        const dx = panAlong(box.left, box.width, room.left, room.width);
        const dy = panAlong(box.top, box.height, room.top, room.height);
        if (dx !== 0 || dy !== 0) {
          const { k } = zoomTransform(element);
          const duration = matchMedia(REDUCED_MOTION_QUERY).matches ? 0 : REVEAL_DURATION;
          zooming.translateBy(select(element).transition().duration(duration), dx / k, dy / k);
        }
      }),
    };
  }, [viewport]);

  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) {
      throw new Error('The canvas renders its viewport before its layout effect runs');
    }
    const selection = select(element);
    const zooming = zoom<HTMLDivElement, unknown>()
      .scaleExtent([MIN_SCALE, MAX_SCALE])
      .clickDistance(CLICK_DISTANCE)
      .wheelDelta(wheelZoom)
      .filter(startsGesture)
      .constrain((next, extent) => keepInSight(next, extent, latestSize.current))
      .on('zoom', (event: D3ZoomEvent<HTMLDivElement, unknown>) => setTransform(event.transform))
      .on('end', (event: D3ZoomEvent<HTMLDivElement, unknown>) => keep(memory, event.transform));
    // A double click enters a node's page; it does not zoom.
    selection.call(zooming).on('dblclick.zoom', null);
    const wheel = (event: WheelEvent) => {
      // The canvas takes every wheel, so the page never zooms, not even when the drawing's zoom is at a bound.
      event.preventDefault();
      if (event.ctrlKey || event.metaKey) {
        return;
      }
      const unit = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? WHEEL_LINE : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? element.clientHeight : 1;
      // A mouse wheel turned with Shift scrolls sideways.
      const [dx, dy] = event.shiftKey && event.deltaX === 0 ? [event.deltaY, 0] : [event.deltaX, event.deltaY];
      const { k } = zoomTransform(element);
      zooming.translateBy(selection, (-dx * unit) / k, (-dy * unit) / k);
    };
    // What the keyboard focuses is panned into view; the viewport clips and does not scroll to it.
    const focus = (event: FocusEvent) => {
      if (event.target instanceof Element && event.target.matches(':focus-visible')) {
        actions.reveal(event.target);
      }
    };
    element.addEventListener('wheel', wheel, { passive: false });
    element.addEventListener('focusin', focus);
    zooming.transform(selection, kept(memory) ?? latestOpening.current(element.clientWidth, element.clientHeight));
    behavior.current = zooming;
    return () => {
      element.removeEventListener('wheel', wheel);
      element.removeEventListener('focusin', focus);
      selection.on('.zoom', null);
      behavior.current = null;
    };
  }, [viewport, memory, actions]);

  return { transform, ...actions };
}
