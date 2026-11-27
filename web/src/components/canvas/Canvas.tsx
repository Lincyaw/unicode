import { Maximize, Minus, Plus } from 'lucide-react';
import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent, type ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import { ICON_SMALL, ICON_STROKE, InfoHint } from '../../ui/hint';
import { REDUCED_MOTION_QUERY } from '../../ui/media';
import { useLinks } from '../marks/links';
import styles from './canvas.module.css';

/** The zoom's bounds and step, and the margin around a drawing. */
const MIN_SCALE = 0.25;
const MAX_SCALE = 2;
const SCALE_STEP = 0.1;

/**
 * A drawing that does not fit its column opens zoomed out toward fitting, but never below READABLE_SCALE, where the
 * smallest text of a card stays legible; the reader pans to the rest.
 */
const READABLE_SCALE = 0.8;
const CANVAS_MARGIN = { top: 52, side: 24 };
const BOTTOM_MARGIN = 72;

/** How close to the viewport's edge the selected node may sit before the canvas brings it into view. */
const EDGE_ROOM = 24;

/**
 * Where the selection is on the drawing: the box of its own mark (a card, a container, a chip), else of a reference to
 * it, such as a type a folded card lists.
 */
function selectedBox(drawing: HTMLElement, selected: string): HTMLElement | null {
  const id = CSS.escape(selected);
  const mark = drawing.querySelector<HTMLElement>(`[data-node="${id}"]:not([data-ref])`) ?? drawing.querySelector<HTMLElement>(`[data-node="${id}"]`);
  return mark && (mark.closest<HTMLElement>('[data-selected]') ?? mark);
}

/**
 * The canvas a view draws on: the drawing at its own size, scrolled and dragged within the column, zoomed from the
 * control at the bottom right around the middle of the view. It opens at a readable scale and brings the selected
 * node into view whenever the selection changes to a node out of sight. Its ⓘ, top left, says how to read the view;
 * `toolbar` sits top right and `footer` bottom left.
 */
export function Canvas({ about, label, width, height, toolbar, footer, margin = CANVAS_MARGIN, aboutBelow = false, children }: {
  about: string; label: string; width: number; height: number; toolbar?: ReactNode; footer?: ReactNode; margin?: { top: number; side: number };
  aboutBelow?: boolean; children: ReactNode;
}) {
  const { selected } = useLinks();
  const viewport = useRef<HTMLDivElement>(null);
  const drawing = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState<number | null>(null);
  // The point of the drawing at the middle of the view, which a zoom keeps in place.
  const anchor = useRef<{ x: number; y: number } | null>(null);
  const fullWidth = width + 2 * margin.side;
  const fullHeight = height + margin.top + BOTTOM_MARGIN;
  const shown = scale ?? 1;

  const zoom = (next: number) => {
    const element = viewport.current;
    if (element) {
      anchor.current = { x: (element.scrollLeft + element.clientWidth / 2) / shown, y: (element.scrollTop + element.clientHeight / 2) / shown };
    }
    setScale(Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.round(next * 100) / 100)));
  };
  const fit = () => {
    const element = viewport.current;
    if (element) {
      zoom(Math.min(1, element.clientWidth / fullWidth, element.clientHeight / fullHeight));
    }
  };

  // The opening scale, once per drawing: a later change of size, such as a card unfolded, keeps the reader's zoom.
  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) {
      throw new Error('The canvas renders its viewport before its layout effect runs');
    }
    if (scale !== null) {
      return;
    }
    const room = Math.min(element.clientWidth / fullWidth, element.clientHeight / fullHeight);
    setScale(room >= 1 ? 1 : Math.max(READABLE_SCALE, Math.floor(room * 20) / 20));
  }, [scale, fullWidth, fullHeight]);

  // A zoom keeps the point that was in the middle of the view there.
  useLayoutEffect(() => {
    const element = viewport.current;
    const point = anchor.current;
    if (element && point) {
      anchor.current = null;
      element.scrollLeft = point.x * shown - element.clientWidth / 2;
      element.scrollTop = point.y * shown - element.clientHeight / 2;
    }
  }, [shown]);

  // A selection out of sight, from the design tree or the address, is brought to the middle of the view.
  const opened = scale !== null;
  useEffect(() => {
    const element = viewport.current;
    const drawn = drawing.current;
    if (!opened || !element || !drawn || selected === undefined) {
      return;
    }
    const mark = selectedBox(drawn, selected);
    if (!mark) {
      return;
    }
    const box = mark.getBoundingClientRect();
    const room = element.getBoundingClientRect();
    const inSight = box.left >= room.left + EDGE_ROOM && box.right <= room.right - EDGE_ROOM && box.top >= room.top + EDGE_ROOM && box.bottom <= room.bottom - EDGE_ROOM;
    if (inSight) {
      return;
    }
    // A box that fits comes to the middle; a larger one shows from its top left corner.
    const along = (start: number, size: number, roomStart: number, roomSize: number) => (
      size <= roomSize - 2 * EDGE_ROOM ? start + size / 2 - (roomStart + roomSize / 2) : start - roomStart - EDGE_ROOM
    );
    element.scrollTo({
      left: element.scrollLeft + along(box.left, box.width, room.left, room.width),
      top: element.scrollTop + along(box.top, box.height, room.top, room.height),
      behavior: matchMedia(REDUCED_MOTION_QUERY).matches ? 'auto' : 'smooth',
    });
  }, [selected, opened]);

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    const element = viewport.current;
    if (!element || (event.target as HTMLElement).closest('button, a, [data-hit]')) {
      return;
    }
    const start = { x: event.clientX, y: event.clientY, left: element.scrollLeft, top: element.scrollTop };
    const move = (moving: globalThis.PointerEvent) => {
      element.scrollLeft = start.left - (moving.clientX - start.x);
      element.scrollTop = start.top - (moving.clientY - start.y);
    };
    const stop = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop);
  };
  return (
    <div className={styles.canvas}>
      <div ref={viewport} className={styles.viewport} onPointerDown={onPointerDown}>
        <div className={styles.sizer} style={{ width: fullWidth * shown, height: fullHeight * shown, visibility: opened ? undefined : 'hidden' }}>
          <div ref={drawing} className={styles.drawing} role="group" aria-label={label} style={{ width, height, transform: `translate(${margin.side * shown}px, ${margin.top * shown}px) scale(${shown})` }}>
            {children}
          </div>
        </div>
      </div>
      <div className={aboutBelow ? styles.aboutBelow : styles.about}><InfoHint label={label}>{about}</InfoHint></div>
      {toolbar && <div className={styles.toolbar}>{toolbar}</div>}
      {footer && <div className={styles.footer}>{footer}</div>}
      <div className={styles.zoom}>
        <button type="button" aria-label={CONTROLS.zoomOut} onClick={() => zoom(shown - SCALE_STEP)}><Minus size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
        <span className="t-mono">{Math.round(shown * 100)}%</span>
        <button type="button" aria-label={CONTROLS.zoomIn} onClick={() => zoom(shown + SCALE_STEP)}><Plus size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
        <button type="button" aria-label={CONTROLS.fit} onClick={fit}><Maximize size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
      </div>
    </div>
  );
}

/** A box placed on the drawing at its rectangle. */
export function Placed({ rect, children, className }: { rect: { x: number; y: number; width: number; height: number }; children: ReactNode; className?: string }) {
  return <div className={className ?? styles.placed} style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height }}>{children}</div>;
}
