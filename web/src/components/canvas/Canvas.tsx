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
 * The canvas a view draws on: the drawing at its own size, panned and zoomed by the reader's gestures (panZoom.ts) and
 * by the control at the bottom right. It opens at a readable scale and brings a node into view when the selection
 * changes to one out of sight, or when a box asks for it, such as a card just unfolded. Its ⓘ, top left, says how to
 * read the view; `toolbar` sits top right and `footer` bottom left.
 */
export function Canvas({ about, label, width, height, toolbar, footer, margin = CANVAS_MARGIN, aboutBelow = false, children }: {
  about: string; label: string; width: number; height: number; toolbar?: ReactNode; footer?: ReactNode; margin?: { top: number; side: number };
  aboutBelow?: boolean; children: ReactNode;
}) {
  const { selected } = useLinks();
  const viewport = useRef<HTMLDivElement>(null);
  const drawing = useRef<HTMLDivElement>(null);
  const size = { width: width + 2 * margin.side, height: height + margin.top + BOTTOM_MARGIN };
  const { transform, zoomTo, fit, reveal } = usePanZoom(viewport, size, (roomWidth, roomHeight) => {
    const room = Math.min(roomWidth / size.width, roomHeight / size.height);
    return zoomIdentity.scale(room >= 1 ? 1 : Math.max(READABLE_SCALE, Math.floor(room * 20) / 20));
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
      <div ref={viewport} className={styles.viewport}>
        <div
          className={styles.sheet}
          style={{ width: size.width, height: size.height, visibility: opened ? undefined : 'hidden', transform: transform ? `translate(${transform.x}px, ${transform.y}px) scale(${transform.k})` : undefined }}
        >
          <div ref={drawing} className={styles.drawing} role="group" aria-label={label} style={{ width, height, left: margin.side, top: margin.top }}>
            <RevealProvider value={id => setAsked({ id })}>{children}</RevealProvider>
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
