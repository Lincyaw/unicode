import { zoomIdentity } from 'd3-zoom';
import { useEffect, useRef, type ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import { FitIcon, IconButton, ZoomIcon } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { useLinks } from '../marks/links';
import { MAX_SCALE, MIN_SCALE, usePanZoom } from './panZoom';
import styles from './canvas.module.css';

/** How far one press of the zoom buttons zooms. */
const SCALE_STEP = 0.1;

/**
 * A drawing that does not fit its column opens zoomed out toward fitting, but never below READABLE_SCALE, where the
 * smallest text of a card stays legible; the reader pans to the rest.
 */
const READABLE_SCALE = 0.8;
const CANVAS_MARGIN = { top: 52, side: 24 };
const BOTTOM_MARGIN = 72;

/** How close to the canvas's edge the selected node may sit before the canvas brings it into view. */
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

/** The pan along one axis that brings a box into the room: to the middle when it fits, else to its start; none when it is in sight. */
function panAlong(start: number, size: number, roomStart: number, roomSize: number): number {
  const inSight = start >= roomStart + EDGE_ROOM && start + size <= roomStart + roomSize - EDGE_ROOM;
  if (inSight) {
    return 0;
  }
  return size <= roomSize - 2 * EDGE_ROOM ? roomStart + roomSize / 2 - (start + size / 2) : roomStart + EDGE_ROOM - start;
}

/**
 * The canvas a view draws on: the drawing at its own size, panned and zoomed by the reader's gestures (panZoom.ts) and
 * by the control at the bottom right. It opens at a readable scale and brings the selected node into view whenever
 * the selection changes to a node out of sight. Its ⓘ, top left, says how to read the view; `toolbar` sits top right
 * and `footer` bottom left.
 */
export function Canvas({ about, label, width, height, toolbar, footer, margin = CANVAS_MARGIN, aboutBelow = false, children }: {
  about: string; label: string; width: number; height: number; toolbar?: ReactNode; footer?: ReactNode; margin?: { top: number; side: number };
  aboutBelow?: boolean; children: ReactNode;
}) {
  const { selected } = useLinks();
  const viewport = useRef<HTMLDivElement>(null);
  const drawing = useRef<HTMLDivElement>(null);
  const size = { width: width + 2 * margin.side, height: height + margin.top + BOTTOM_MARGIN };
  const { transform, zoomTo, fit, panBy } = usePanZoom(viewport, size, (roomWidth, roomHeight) => {
    const room = Math.min(roomWidth / size.width, roomHeight / size.height);
    return zoomIdentity.scale(room >= 1 ? 1 : Math.max(READABLE_SCALE, Math.floor(room * 20) / 20));
  });
  const scale = transform?.k ?? 1;
  const step = (direction: 1 | -1) => zoomTo(Math.round((scale + direction * SCALE_STEP) * 100) / 100);

  // A selection out of sight, from the design tree or the address, is brought into view.
  const opened = transform !== null;
  useEffect(() => {
    const room = viewport.current;
    const drawn = drawing.current;
    const mark = opened && room && drawn && selected !== undefined ? selectedBox(drawn, selected) : null;
    if (!room || !mark) {
      return;
    }
    const box = mark.getBoundingClientRect();
    const bounds = room.getBoundingClientRect();
    const dx = panAlong(box.left, box.width, bounds.left, bounds.width);
    const dy = panAlong(box.top, box.height, bounds.top, bounds.height);
    if (dx !== 0 || dy !== 0) {
      panBy(dx, dy);
    }
  }, [selected, opened, panBy]);

  return (
    <div className={styles.canvas}>
      <div ref={viewport} className={styles.viewport}>
        <div
          className={styles.sheet}
          style={{ width: size.width, height: size.height, visibility: opened ? undefined : 'hidden', transform: transform ? `translate(${transform.x}px, ${transform.y}px) scale(${transform.k})` : undefined }}
        >
          <div ref={drawing} className={styles.drawing} role="group" aria-label={label} style={{ width, height, left: margin.side, top: margin.top }}>
            {children}
          </div>
        </div>
      </div>
      <div className={aboutBelow ? styles.aboutBelow : styles.about}><InfoHint label={label}>{about}</InfoHint></div>
      {toolbar && <div className={styles.toolbar}>{toolbar}</div>}
      {footer && <div className={styles.footer}>{footer}</div>}
      <div className={styles.zoom}>
        <IconButton label={CONTROLS.zoomOut} size="small" variant="quiet" disabled={scale <= MIN_SCALE} onClick={() => step(-1)}><ZoomIcon direction="out" /></IconButton>
        <span className="t-mono">{Math.round(scale * 100)}%</span>
        <IconButton label={CONTROLS.zoomIn} size="small" variant="quiet" disabled={scale >= MAX_SCALE} onClick={() => step(1)}><ZoomIcon direction="in" /></IconButton>
        <IconButton label={CONTROLS.fit} size="small" variant="quiet" onClick={fit}><FitIcon /></IconButton>
      </div>
    </div>
  );
}

/** A box placed on the drawing at its rectangle. */
export function Placed({ rect, children, className }: { rect: { x: number; y: number; width: number; height: number }; children: ReactNode; className?: string }) {
  return <div className={className ?? styles.placed} style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height }}>{children}</div>;
}
