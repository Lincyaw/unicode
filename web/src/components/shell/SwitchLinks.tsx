import { clsx } from 'clsx';
import { useLayoutEffect, useRef, useState } from 'react';
import { Link } from 'react-router';
import styles from './shell.module.css';

export interface SwitchItem {
  key: string;
  label: string;

  /** Where it leads; an item without one cannot be chosen now and stays faint */
  href?: string;
}

/**
 * A row of mutually exclusive choices that are places, such as the view tabs or a view's layer: each is a link, and a
 * thumb under the current one slides to the next one chosen.
 */
export function SwitchLinks({ label, items, current, className }: { label: string; items: SwitchItem[]; current?: string; className?: string }) {
  const track = useRef<HTMLElement>(null);
  const [thumb, setThumb] = useState<{ left: number; width: number } | null>(null);
  useLayoutEffect(() => {
    const trackElement = track.current;
    if (!trackElement) {
      throw new Error('A switch renders its track before its layout effect runs');
    }
    const place = () => {
      const item = current === undefined ? null : trackElement.querySelector<HTMLElement>(`[data-key="${CSS.escape(current)}"]`);
      setThumb(item ? { left: item.offsetLeft, width: item.offsetWidth } : null);
    };
    place();
    const observer = new ResizeObserver(place);
    trackElement.querySelectorAll('[data-key]').forEach(item => observer.observe(item));
    return () => observer.disconnect();
  }, [current]);
  return (
    <nav ref={track} className={clsx(styles.tabs, className)} aria-label={label}>
      {thumb && <span className={styles.thumb} style={{ width: thumb.width, transform: `translateX(${thumb.left}px)` }} aria-hidden="true" />}
      {items.map(item => (item.href === undefined
        ? <span key={item.key} data-key={item.key} className={clsx(styles.tab, styles.unavailable)} aria-disabled="true">{item.label}</span>
        : <Link key={item.key} data-key={item.key} className={styles.tab} to={item.href} aria-current={item.key === current ? 'page' : undefined}>{item.label}</Link>))}
    </nav>
  );
}
