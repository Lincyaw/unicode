import type { KeyboardEvent, PointerEvent } from 'react';
import styles from './resizer.module.css';

/*
 * Columns the reader sizes, as in an editor: a thin handle between two columns drags the one it belongs to wider or
 * narrower, the arrow keys move it a step at a time, and a double click puts it back. useStoredSize keeps the size.
 */

/** How far one press of an arrow key moves a handle. */
export const RESIZE_STEP = 16;

/**
 * A handle that sizes the column on one side of it. `grows` says which way a drag widens the column: toward the end for a
 * column before the handle, toward the start for one after it.
 */
export function ResizeHandle({ label, size, min, max, grows, onResize, onReset }: {
  label: string;
  size: number;
  min: number;
  max: number;
  grows: 'end' | 'start';
  onResize: (size: number) => void;
  onReset: () => void;
}) {
  const sign = grows === 'end' ? 1 : -1;
  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    const handle = event.currentTarget;
    const [origin, from] = [event.clientX, size];
    handle.setPointerCapture(event.pointerId);
    const move = (moving: globalThis.PointerEvent) => onResize(from + sign * (moving.clientX - origin));
    const stop = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', stop);
      handle.removeEventListener('pointercancel', stop);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const direction = { ArrowRight: 1, ArrowLeft: -1 }[event.key];
    if (direction !== undefined) {
      event.preventDefault();
      onResize(size + sign * direction * RESIZE_STEP);
    }
  };
  return (
    <div
      className={styles.handle} role="separator" aria-orientation="vertical" aria-label={label} aria-valuenow={Math.round(size)} aria-valuemin={min} aria-valuemax={max}
      tabIndex={0} onPointerDown={onPointerDown} onKeyDown={onKeyDown} onDoubleClick={onReset}
    />
  );
}
