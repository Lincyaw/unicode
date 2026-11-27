import type { ReactNode } from 'react';
import type { Route } from './layout';
import styles from './canvas.module.css';

/**
 * How a line is drawn: plain, dashed, a warning, a reaches edge (dashed in the information colour), the selected line
 * (solid and heavier), an event's colour, or faint.
 */
export type LineTone = 'plain' | 'dashed' | 'warning' | 'reach' | 'chosen' | 'event' | 'faint';

export interface Line {
  id: string;
  route: Route;
  tone: LineTone;

  /** No arrowhead, for a line that only joins */
  plain?: boolean;

  /** The edge it draws, when it previews on a click: `kind from to` */
  select?: string;
}

const TONES: LineTone[] = ['plain', 'dashed', 'warning', 'reach', 'chosen', 'event', 'faint'];

/**
 * The lines of a drawing, under its boxes, with an open arrowhead at the end each points to. A line that draws an edge
 * with a preview is a control: a click or Enter previews the edge, and it takes the keyboard's focus.
 */
export function Lines({ width, height, lines, selected, onSelect }: {
  width: number; height: number; lines: Line[]; selected?: string; onSelect?: (key: string) => void;
}) {
  return (
    <svg className={styles.lines} width={width} height={height}>
      <defs aria-hidden="true">
        {TONES.map(tone => (
          <marker key={tone} id={`arrow-${tone}`} viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse" markerUnits="userSpaceOnUse">
            <path d="M1 1 L7 4 L1 7" className={styles.arrow} data-tone={tone} />
          </marker>
        ))}
      </defs>
      {lines.map(line => {
        const tone = line.select !== undefined && line.select === selected ? 'chosen' : line.tone;
        const key = line.select;
        return (
          <g key={line.id}>
            {/* The target goes under the line it draws, so that its focus shows as a band on either side of the line. */}
            {key !== undefined && onSelect && (
              <path
                d={line.route.d} className={styles.hit} data-hit role="button" tabIndex={0} aria-label={key} aria-pressed={key === selected}
                onClick={() => onSelect(key)}
                onKeyDown={event => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    onSelect(key);
                  }
                }}
              />
            )}
            <path d={line.route.d} className={styles.line} data-tone={tone} markerEnd={line.plain ? undefined : `url(#arrow-${tone})`} aria-hidden="true" />
          </g>
        );
      })}
    </svg>
  );
}

/** A label or badge placed at a point of the drawing: centred on it, beside it to the right or left, or above it. */
export function Marker({ at, side = 'center', children }: { at: { x: number; y: number }; side?: 'center' | 'right' | 'left' | 'above'; children: ReactNode }) {
  return <div className={styles.marker} data-side={side} style={{ left: at.x, top: at.y }}>{children}</div>;
}
