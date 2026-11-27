import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { InfoHint } from './hint';
import styles from './content.module.css';
import { UI_WORDS } from './words';

/**
 * What a tag's edge says: nothing beyond its words, or a real status, a failure, a success or something to heed, in the
 * status colour; the words keep the text colour, so they read as well as any other.
 */
export type TagTone = 'neutral' | 'warning' | 'success' | 'caution';

/** A label that is not clickable, such as a version, a count or a state, optionally led by an icon. */
export function Tag({ children, dashed = false, tone = 'neutral' }: { children: ReactNode; dashed?: boolean; tone?: TagTone }) {
  return <span className={clsx(styles.tag, dashed && styles.dashed, 't-micro')} data-tone={tone}>{children}</span>;
}

/**
 * A heading of the given rank and text style with, beside it, the ⓘ that holds what explains the part it heads, such
 * as how its figures are counted; a heading with nothing to explain stands alone.
 */
export function HintedHeading({ rank: Heading, className, title, about }: { rank: 'h1' | 'h2' | 'h3'; className: string; title: string; about?: ReactNode }) {
  return (
    <div className={styles.hinted}>
      <Heading className={className}>{title}</Heading>
      {about && <InfoHint subject={title}>{about}</InfoHint>}
    </div>
  );
}

export function SectionHeading({ id, title, about }: { id?: string; title: string; about?: ReactNode }) {
  return (
    <header className={styles.sectionHeading} id={id}>
      <HintedHeading rank="h2" className="t-title" title={title} about={about} />
    </header>
  );
}

export interface Fact {
  label: string;
  value: ReactNode;

  /** Code, such as a repository path, is set in the monospace face */
  format?: 'text' | 'code';
}

/**
 * Labelled values in one of four arrangements: stacked label over value in a flowing row, two-column rows, label and
 * value side by side in a flowing line, or pairs in a narrow column, the names as wide as the widest. A label may
 * repeat, such as several call sites under one name.
 */
export function FactList({ facts, layout }: { facts: Fact[]; layout: 'stacked' | 'rows' | 'inline' | 'pairs' }) {
  return (
    <dl className={styles[layout]}>
      {facts.map((fact, index) => (
        <div key={`${index} ${fact.label}`}>
          <dt className={clsx(layout === 'rows' || layout === 'pairs' ? 't-small c-muted' : 't-micro c-faint')}>{fact.label}</dt>
          <dd className={fact.format === 'code' ? 't-mono' : 't-small'}>{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * One figure of a page's headline figures: its name with the ⓘ that says how it is counted, the figure, and a figure
 * beside it such as what it leaves out.
 */
export function StatTile({ label, value, about, note }: { label: string; value: ReactNode; about?: ReactNode; note?: string }) {
  return (
    <div className={styles.stat}>
      <dt className={clsx(styles.statLabel, 't-micro c-muted')}>{label}{about && <InfoHint subject={label}>{about}</InfoHint>}</dt>
      <dd className={styles.statValue}>{value}</dd>
      {note && <dd className="t-micro c-faint">{note}</dd>}
    </div>
  );
}

/** A row of stat tiles that wraps on narrow screens. */
export function StatTiles({ children }: { children: ReactNode }) {
  return <dl className={styles.stats}>{children}</dl>;
}

export function Loading() {
  return <p className={clsx(styles.loading, 't-small c-faint')} role="status">{UI_WORDS.loading}</p>;
}
