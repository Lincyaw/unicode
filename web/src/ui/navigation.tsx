import { clsx } from 'clsx';
import { useLayoutEffect, useRef, type HTMLAttributes, type ReactNode } from 'react';
import { Link } from 'react-router';
import { ArrowIcon, IconButton } from './controls';
import styles from './navigation.module.css';
import { UI_WORDS } from './words';

export interface Crumb {
  label: string;

  /** Site path of the crumb; the last crumb, the current page, has none */
  to?: string;
}

/** The pages above the current one; each leads back to its page. */
export function Breadcrumbs({ crumbs, className }: { crumbs: Crumb[]; className?: string }) {
  return (
    <nav className={clsx(styles.crumbs, 't-small', className)} aria-label={UI_WORDS.breadcrumbs}>
      {crumbs.map((crumb, index) => (
        <span key={crumb.label}>
          {index > 0 && <span className={styles.slash}>/</span>}
          {crumb.to ? <Link to={crumb.to}>{crumb.label}</Link> : <span aria-current="page" className="c-main">{crumb.label}</span>}
        </span>
      ))}
    </nav>
  );
}

/**
 * A row of tabs with one ∞ gradient bar under the current tab, which slides when the current tab changes. Each child
 * marks itself with `data-tab`; `current` names the child the bar sits under.
 */
export function TabRow({ current, className, children, ...attributes }: { current: string; className?: string; children: ReactNode } & HTMLAttributes<HTMLDivElement>) {
  const row = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const rowElement = row.current;
    const barElement = bar.current;
    if (!rowElement || !barElement) {
      throw new Error('The tab row renders its elements before its layout effect runs');
    }
    const place = () => {
      const tab = rowElement.querySelector<HTMLElement>(`[data-tab="${CSS.escape(current)}"]`);
      barElement.style.opacity = tab ? '1' : '0';
      barElement.style.width = `${tab?.offsetWidth ?? 0}px`;
      barElement.style.transform = `translateX(${tab?.offsetLeft ?? 0}px)`;
    };
    place();
    // Tabs change width when fonts or counts arrive.
    const observer = new ResizeObserver(place);
    Array.from(rowElement.children).forEach(child => observer.observe(child));
    return () => observer.disconnect();
  }, [current]);
  return (
    <div ref={row} className={clsx(styles.tabs, className)} {...attributes}>
      {children}
      <span ref={bar} className={styles.indicator} aria-hidden="true" />
    </div>
  );
}

/** Turns a list a page back or forth, saying which items are in view; an end it has reached cannot be passed. */
export function Pager({ label, atStart, atEnd, onPrevious, onNext, className }: { label: string; atStart: boolean; atEnd: boolean; onPrevious: () => void; onNext: () => void; className?: string }) {
  return (
    <div className={clsx(styles.pager, className)}>
      <IconButton label={UI_WORDS.previousPage} disabled={atStart} onClick={onPrevious}><ArrowIcon direction="left" /></IconButton>
      <span className="t-micro c-muted">{label}</span>
      <IconButton label={UI_WORDS.nextPage} disabled={atEnd} onClick={onNext}><ArrowIcon direction="right" /></IconButton>
    </div>
  );
}

export interface PageSection {
  id: string;
  label: string;
}

/** Links to the sections of the page being read, with the one in view marked. */
export function SectionLinks({ sections, current, className }: { sections: PageSection[]; current: string; className?: string }) {
  return (
    <TabRow current={current} className={className}>
      {sections.map(section => (
        <a key={section.id} href={`#${section.id}`} data-tab={section.id} className={clsx(styles.tab, 't-small')} aria-current={section.id === current ? 'true' : undefined}>{section.label}</a>
      ))}
    </TabRow>
  );
}
