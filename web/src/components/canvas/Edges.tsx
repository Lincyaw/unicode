import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import type { Rect, Route } from './layout';
import { placeSpots, type Side } from './markers';
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

/** A label or badge of a line: what it shows, and the sides of the line it accepts, the preferred first. */
export interface LineMark {
  id: string;
  route: Route;
  sides: readonly Side[];
  children: ReactNode;
}

/**
 * The labels and badges of a drawing's lines, each placed where it covers none of the boxes (`obstacles`) and no other
 * mark (markers.ts). They are measured as drawn, then placed before the drawing shows.
 */
export function LineMarks({ marks, obstacles }: { marks: LineMark[]; obstacles: Rect[] }) {
  const elements = useRef(new Map<string, HTMLDivElement>());
  const [sizes, setSizes] = useState(new Map<string, { width: number; height: number }>());
  useLayoutEffect(() => {
    const measured = new Map([...elements.current].map(([id, element]) => [id, { width: element.offsetWidth, height: element.offsetHeight }]));
    setSizes(known => {
      const same = measured.size === known.size && [...measured].every(([id, size]) => known.get(id)?.width === size.width && known.get(id)?.height === size.height);
      return same ? known : measured;
    });
  }, [marks]);
  const places = placeSpots(marks.flatMap(mark => {
    const size = sizes.get(mark.id);
    return size ? [{ id: mark.id, route: mark.route, sides: mark.sides, ...size }] : [];
  }), obstacles);
  return marks.map(mark => {
    const place = places.get(mark.id);
    return (
      <div
        key={mark.id}
        ref={element => {
          if (element) {
            elements.current.set(mark.id, element);
          } else {
            elements.current.delete(mark.id);
          }
        }}
        className={styles.marker} style={place ? { left: place.x, top: place.y } : { left: mark.route.mid.x, top: mark.route.mid.y, visibility: 'hidden' }}
      >
        {mark.children}
      </div>
    );
  });
}
