import { Maximize, Minus, Plus } from 'lucide-react';
import { useRef, useState, type PointerEvent, type ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import { ICON_SMALL, ICON_STROKE, InfoHint } from '../../ui/hint';
import styles from './canvas.module.css';

/** The zoom's bounds and step, and the margin around a drawing. */
const MIN_SCALE = 0.25;
const MAX_SCALE = 2;
const SCALE_STEP = 0.1;
const CANVAS_MARGIN = { top: 52, side: 24 };
const BOTTOM_MARGIN = 72;

/**
 * The canvas a view draws on: the drawing at its own size, scrolled and dragged within the column, zoomed from the
 * control at the bottom right. Its ⓘ, top left, says how to read the view; `toolbar` sits top right and `footer`
 * bottom left.
 */
export function Canvas({ about, label, width, height, toolbar, footer, margin = CANVAS_MARGIN, aboutBelow = false, children }: {
  about: string; label: string; width: number; height: number; toolbar?: ReactNode; footer?: ReactNode; margin?: { top: number; side: number };
  aboutBelow?: boolean; children: ReactNode;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const zoom = (next: number) => setScale(Math.min(MAX_SCALE, Math.max(MIN_SCALE, Math.round(next * 100) / 100)));
  const fullWidth = width + 2 * margin.side;
  const fullHeight = height + margin.top + BOTTOM_MARGIN;
  const fit = () => {
    const element = viewport.current;
    if (element) {
      zoom(Math.min(1, element.clientWidth / fullWidth, element.clientHeight / fullHeight));
    }
  };
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
        <div className={styles.sizer} style={{ width: fullWidth * scale, height: fullHeight * scale }}>
          <div className={styles.drawing} role="group" aria-label={label} style={{ width, height, transform: `translate(${margin.side * scale}px, ${margin.top * scale}px) scale(${scale})` }}>
            {children}
          </div>
        </div>
      </div>
      <div className={aboutBelow ? styles.aboutBelow : styles.about}><InfoHint label={label}>{about}</InfoHint></div>
      {toolbar && <div className={styles.toolbar}>{toolbar}</div>}
      {footer && <div className={styles.footer}>{footer}</div>}
      <div className={styles.zoom}>
        <button type="button" aria-label={CONTROLS.zoomOut} onClick={() => zoom(scale - SCALE_STEP)}><Minus size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
        <span className="t-mono">{Math.round(scale * 100)}%</span>
        <button type="button" aria-label={CONTROLS.zoomIn} onClick={() => zoom(scale + SCALE_STEP)}><Plus size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
        <button type="button" aria-label={CONTROLS.fit} onClick={fit}><Maximize size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" /></button>
      </div>
    </div>
  );
}

/** A box placed on the drawing at its rectangle. */
export function Placed({ rect, children, className }: { rect: { x: number; y: number; width: number; height: number }; children: ReactNode; className?: string }) {
  return <div className={className ?? styles.placed} style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height }}>{children}</div>;
}
