import { clsx } from 'clsx';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type { CSSProperties, KeyboardEvent, ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node, ViewNode } from '../../api/types';
import { ABOUT, CONTROLS, kindWord } from '../../terms';
import { ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import { opLine } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { StatusDot } from '../marks/StatusDot';
import type { CardRow } from './sizes';
import styles from './cards.module.css';

/*
 * The boxes the node-and-edge views draw (the prototype's type cards, node chips and container cards): each is sized here from its
 * content, so the layout can place it before it is drawn, and previews its node on a click and enters its page on a
 * double click.
 */

/** Makes a box preview its node on a click, enter its page on a double click, and do the same from the keyboard. */
function useBoxActions(id: string) {
  const links = useLinks();
  return {
    selected: links.selected === id,
    props: {
      role: 'button',
      tabIndex: 0,
      onClick: () => links.select(id),
      onDoubleClick: () => links.open(id),
      onKeyDown: (event: KeyboardEvent) => {
        if (event.key === 'Enter') {
          links.open(id);
        } else if (event.key === ' ') {
          event.preventDefault();
          links.select(id);
        }
      },
    },
  };
}

/** The ⓘ of a box: what the node is, its form and where it is written, and its summary. */
function NodeHint({ node }: { node: Node }) {
  return (
    <span className={styles.hint} onClick={event => event.stopPropagation()} onKeyDown={event => event.stopPropagation()} role="presentation">
      <InfoHint label={node.id}>
        <span>{ABOUT.card(kindWord(node.kind, node.form, node.layer), node.form, `${node.origin.path}:${node.origin.line}`)}</span>
        {node.summary !== '' && <span>{node.summary}</span>}
      </InfoHint>
    </span>
  );
}

function Rows({ rows }: { rows: CardRow[] }) {
  return rows.map((row, index) => {
    switch (row.kind) {
      case 'field':
        return (
          <span key={index} className={styles.field} data-warn={row.warn}>
            <span>{row.name}</span>
            <span className={styles.fieldType}>{row.type}</span>
          </span>
        );
      case 'value':
        return <span key={index} className={styles.field} data-warn={row.warn}><span>{row.name}</span></span>;
      case 'op':
        return <span key={index} className={styles.op} title={row.text}>{row.text}</span>;
      case 'more':
        return <span key={index} className={clsx(styles.field, styles.more)}>+{row.count}</span>;
    }
  });
}

/** A design type drawn as a card: its fields or values and its operations; a contradicted type has a red edge. */
export function TypeCard({ node, rows, style }: { node: ViewNode; rows: CardRow[]; style?: CSSProperties }) {
  const box = useBoxActions(node.id);
  return (
    <div className={styles.card} data-status={node.status} aria-pressed={box.selected} style={style} {...box.props}>
      <span className={styles.head}>
        <StatusDot status={node.status} />
        <KindIcon node={node} size={ICON_SMALL} />
        <span className={styles.title}>{node.title}</span>
        <NodeHint node={node} />
      </span>
      <Rows rows={rows} />
    </div>
  );
}

/**
 * A container drawn folded: its name and the names of what it holds; the chevron expands it in place. A count, such
 * as how many edges an edge into it stands for, can follow the card.
 */
export function FoldedCard({ node, names, style }: { node: ViewNode; names: ViewNode[]; style?: CSSProperties }) {
  const links = useLinks();
  const box = useBoxActions(node.id);
  const expandHref = links.expandHref?.(node.id);
  return (
    <div className={clsx(styles.card, styles.folded)} aria-pressed={box.selected} style={style} {...box.props}>
      <span className={styles.head}>
        <StatusDot status={node.status} />
        {expandHref && (
          <Link className={styles.toggle} to={expandHref} aria-label={CONTROLS.expandRow} onClick={event => event.stopPropagation()}>
            <ChevronRight size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
          </Link>
        )}
        <KindIcon node={node} size={ICON_SMALL} />
        <span className={clsx(styles.title, styles.mutedTitle)}>{node.title}</span>
      </span>
      {names.length > 0 && (
        <span className={styles.names}>
          {names.map(child => (
            <span key={child.id} className={styles.name}>
              <KindIcon node={child} size={ICON_TINY} />
              {child.title}
            </span>
          ))}
        </span>
      )}
    </div>
  );
}

/** An external system drawn with a dashed edge: its operations' signatures. */
export function ExternalCard({ node, style }: { node: ViewNode; style?: CSSProperties }) {
  const box = useBoxActions(node.id);
  const ops = (node.contents ?? []).filter(child => child.kind === 'op');
  return (
    <div className={clsx(styles.card, styles.external)} aria-pressed={box.selected} style={style} {...box.props}>
      <span className={styles.head}>
        <KindIcon node={node} size={ICON_SMALL} />
        <span className={styles.title}>{node.title}</span>
      </span>
      {ops.map(op => <span key={op.id} className={styles.op}>{opLine(op)}</span>)}
    </div>
  );
}

/**
 * An expanded container: a box with its head, holding what the view lays out inside it. The chevron collapses it when
 * the view expanded it on request.
 */
export function ContainerBox({ node, style, collapsible, children }: { node: ViewNode; style?: CSSProperties; collapsible: boolean; children: ReactNode }) {
  const links = useLinks();
  const box = useBoxActions(node.id);
  const expandHref = collapsible ? links.expandHref?.(node.id) : undefined;
  return (
    <div className={styles.container} aria-pressed={box.selected} style={style}>
      <span className={styles.containerHead} {...box.props}>
        <StatusDot status={node.status} />
        {expandHref && (
          <Link className={styles.toggle} to={expandHref} aria-label={CONTROLS.collapseRow} onClick={event => event.stopPropagation()}>
            <ChevronDown size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
          </Link>
        )}
        <KindIcon node={node} size={ICON_SMALL} />
        <span className={styles.title}>{node.title}</span>
        <NodeHint node={node} />
      </span>
      {children}
    </div>
  );
}

