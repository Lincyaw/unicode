import { zoomIdentity } from 'd3-zoom';
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import { FitIcon, IconButton, ZoomIcon } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { useLinks } from '../marks/links';
import { MAX_SCALE, MIN_SCALE, RevealProvider, usePanZoom } from './panZoom';
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

/**
 * Where a node is on the drawing: the box of its own mark (a selected card or component), else its mark (a chip), else
 * a reference to it, such as a type a folded card lists.
 */
function boxOf(drawing: HTMLElement, id: string): HTMLElement | null {
  const key = CSS.escape(id);
  const mark = drawing.querySelector<HTMLElement>(`[data-node="${key}"]:not([data-ref])`) ?? drawing.querySelector<HTMLElement>(`[data-node="${key}"]`);
  return mark && (mark.closest<HTMLElement>('[data-selected="true"]') ?? mark);
}

/**
 * The point of the sheet a drawing opens centred on: the selected node's box when the drawing shows it; else the whole
 * drawing's middle when it fits the canvas at the opening scale; else the middle of the box nearest the drawing's
 * middle, so that the canvas opens on content.
 */
function openingCentre(sheet: HTMLElement, drawing: HTMLElement, selected: string | undefined, fits: boolean): { x: number; y: number } {
  const frame = sheet.getBoundingClientRect();
  const scale = frame.width / sheet.offsetWidth;
  const middleOf = (element: Element) => {
    const rect = element.getBoundingClientRect();
    return { x: (rect.left + rect.width / 2 - frame.left) / scale, y: (rect.top + rect.height / 2 - frame.top) / scale };
  };
  const chosen = selected === undefined ? null : boxOf(drawing, selected);
  if (chosen) {
    return middleOf(chosen);
  }
  const middle = { x: sheet.offsetWidth / 2, y: sheet.offsetHeight / 2 };
  if (fits) {
    return middle;
  }
  const boxes = [...drawing.querySelectorAll('[data-node]')].map(middleOf);
  return boxes.reduce((best, box) => (Math.hypot(box.x - middle.x, box.y - middle.y) < Math.hypot(best.x - middle.x, best.y - middle.y) ? box : best), boxes[0] ?? middle);
}

/** Where a canvas's ⓘ stands: top left, or bottom left for a drawing whose heads run along its top. */
export type AboutSide = 'top' | 'bottom';

/**
 * The canvas a view draws on: the drawing at its own size, panned and zoomed by the reader's gestures (panZoom.ts) and
 * by the control at the bottom right. It opens at a readable scale, centred on the selection or on the drawing's
 * content (openingCentre), and brings a node into view when the selection changes to one out of sight, or when a box
 * asks for it, such as a card just unfolded. Its ⓘ, top left, says how to read the view; `toolbar` sits top right and
 * `footer` bottom left.
 */
export function Canvas({ about, label, width, height, toolbar, footer, margin = CANVAS_MARGIN, aboutSide = 'top', children }: {
  about: string; label: string; width: number; height: number; toolbar?: ReactNode; footer?: ReactNode; margin?: { top: number; side: number };
  aboutSide?: AboutSide; children: ReactNode;
}) {
  const { selected } = useLinks();
  const viewport = useRef<HTMLDivElement>(null);
  const sheet = useRef<HTMLDivElement>(null);
  const drawing = useRef<HTMLDivElement>(null);
  const size = { width: width + 2 * margin.side, height: height + margin.top + BOTTOM_MARGIN };
  const { transform, zoomTo, fit, reveal } = usePanZoom(viewport, size, (roomWidth, roomHeight) => {
    const room = Math.min(roomWidth / size.width, roomHeight / size.height);
    const scale = room >= 1 ? 1 : Math.max(READABLE_SCALE, Math.floor(room * 20) / 20);
    if (!sheet.current || !drawing.current) {
      throw new Error('A canvas opens its drawing once the sheet and the drawing are rendered');
    }
    const centre = openingCentre(sheet.current, drawing.current, selected, room >= scale);
    return zoomIdentity.translate(roomWidth / 2 - centre.x * scale, roomHeight / 2 - centre.y * scale).scale(scale);
  });
  const scale = transform?.k ?? 1;
  const step = (direction: 1 | -1) => zoomTo(Math.round((scale + direction * SCALE_STEP) * 100) / 100);
  const opened = transform !== null;

  const bring = useCallback((id: string | undefined) => {
    const box = opened && drawing.current && id !== undefined ? boxOf(drawing.current, id) : null;
    if (box) {
      reveal(box);
    }
  }, [opened, reveal]);
  // The selection, when it changes to a node out of sight; and the node a box asked to see, such as a card just
  // unfolded, once the drawing has redrawn around it.
  const [asked, setAsked] = useState<{ id: string } | null>(null);
  useEffect(() => bring(selected), [selected, bring]);
  useEffect(() => bring(asked?.id), [asked, bring]);

  return (
    <div className={styles.canvas}>
      <div ref={viewport} className={styles.viewport} data-viewport>
        <div
          ref={sheet}
          className={styles.sheet}
          data-sheet
          style={{ width: size.width, height: size.height, visibility: opened ? undefined : 'hidden', transform: transform ? `translate(${transform.x}px, ${transform.y}px) scale(${transform.k})` : undefined }}
        >
          <div ref={drawing} className={styles.drawing} role="group" aria-label={label} data-drawing style={{ width, height, left: margin.side, top: margin.top }}>
            <RevealProvider value={id => setAsked({ id })}>{children}</RevealProvider>
          </div>
        </div>
      </div>
      <div className={styles.about} data-side={aboutSide}><InfoHint subject={label}>{about}</InfoHint></div>
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
