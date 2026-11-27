import { clsx } from 'clsx';
import { ChevronDown, ChevronRight, ChevronUp } from 'lucide-react';
import type { CSSProperties, ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node, ViewNode } from '../../api/types';
import { ABOUT, CONTROLS, kindWord } from '../../terms';
import { ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import type { TypePart } from '../marks/format';
import { opLine } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { Above, CoverButton, NodeRef } from '../marks/NodeButton';
import { StatusDot } from '../marks/StatusDot';
import type { CardRow } from './sizes';
import styles from './cards.module.css';

/*
 * The boxes the node-and-edge views draw (the prototype's type cards, node chips and container cards): each is sized in
 * sizes.ts from its content, so the layout can place it before it is drawn. A box's head covers the box: a click on it
 * previews its node, a double click enters its page. Every node a box names besides its own (a field's type, an
 * operation, a type a folded container holds) is a reference of its own over the cover.
 */

/** The ⓘ of a box: what the node is, its form and where it is written, and its summary. */
function NodeHint({ node }: { node: Node }) {
  return (
    <Above>
      <InfoHint label={node.id}>
        <span>{ABOUT.card(kindWord(node.kind, node.form, node.layer), node.form, `${node.origin.path}:${node.origin.line}`)}</span>
        {node.summary !== '' && <span>{node.summary}</span>}
      </InfoHint>
    </Above>
  );
}

/** A written type with each node it names as a reference. */
export function TypeRefs({ parts }: { parts: TypePart[] }) {
  return parts.map((part, index) => (part.id === undefined ? part.text : <NodeRef key={index} id={part.id}>{part.text}</NodeRef>));
}

function Rows({ rows, onToggle }: { rows: CardRow[]; onToggle?: () => void }) {
  return rows.map((row, index) => {
    switch (row.kind) {
      case 'field':
        return (
          <span key={index} className={styles.field} data-warn={row.warn}>
            <span>{row.name}</span>
            <span className={styles.fieldType}><TypeRefs parts={row.type} /></span>
          </span>
        );
      case 'value':
        return <span key={index} className={styles.field} data-warn={row.warn}><span>{row.name}</span></span>;
      case 'op':
        return <NodeRef key={index} id={row.op.id} className={styles.op}>{row.text}</NodeRef>;
      case 'more':
        return <button key={index} type="button" className={clsx(styles.field, styles.more)} aria-expanded="false" onClick={onToggle}>+{row.count}</button>;
      case 'less':
        return (
          <button key={index} type="button" className={clsx(styles.field, styles.more)} aria-expanded="true" aria-label={CONTROLS.collapseRow} onClick={onToggle}>
            <ChevronUp size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
          </button>
        );
    }
  });
}

/**
 * A design type drawn as a card: its fields or values and its operations; a contradicted type has a red edge. Its +n
 * unfolds the card to all its fields in place, `onToggle` telling the drawing to make room.
 */
export function TypeCard({ node, rows, onToggle, style }: { node: ViewNode; rows: CardRow[]; onToggle?: () => void; style?: CSSProperties }) {
  const links = useLinks();
  return (
    <div className={styles.card} data-status={node.status} data-selected={links.selected === node.id} style={style}>
      <span className={styles.head}>
        <CoverButton id={node.id}>
          <StatusDot status={node.status} />
          <KindIcon node={node} size={ICON_SMALL} />
          <span className={styles.title}>{node.title}</span>
        </CoverButton>
        <NodeHint node={node} />
      </span>
      <Rows rows={rows} onToggle={onToggle} />
    </div>
  );
}

/** The chevron that expands or collapses a container in place, lifted over the box's cover. */
function Toggle({ href, open }: { href: string; open: boolean }) {
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <Above>
      <Link className={styles.toggle} to={href} aria-label={open ? CONTROLS.collapseRow : CONTROLS.expandRow}>
        <Chevron size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
      </Link>
    </Above>
  );
}

/**
 * A container drawn folded: its name and the names of what it holds, each a reference to its node; the chevron
 * expands it in place. A count, such as how many edges an edge into it stands for, can follow the card.
 */
export function FoldedCard({ node, names, style }: { node: ViewNode; names: ViewNode[]; style?: CSSProperties }) {
  const links = useLinks();
  const expandHref = links.expandHref?.(node.id);
  return (
    <div className={clsx(styles.card, styles.folded)} data-selected={links.selected === node.id} style={style}>
      <span className={styles.head}>
        <StatusDot status={node.status} />
        {expandHref && <Toggle href={expandHref} open={false} />}
        <CoverButton id={node.id}>
          <KindIcon node={node} size={ICON_SMALL} />
          <span className={clsx(styles.title, styles.mutedTitle)}>{node.title}</span>
        </CoverButton>
      </span>
      {names.length > 0 && (
        <span className={styles.names}>
          {names.map(child => (
            <NodeRef key={child.id} id={child.id} className={styles.name}>
              <KindIcon node={child} size={ICON_TINY} />
              {child.title}
            </NodeRef>
          ))}
        </span>
      )}
    </div>
  );
}

/** An external system drawn with a dashed edge: its operations' signatures, each a reference to its operation. */
export function ExternalCard({ node, style }: { node: ViewNode; style?: CSSProperties }) {
  const links = useLinks();
  const ops = (node.contents ?? []).filter(child => child.kind === 'op');
  return (
    <div className={clsx(styles.card, styles.external)} data-selected={links.selected === node.id} style={style}>
      <span className={styles.head}>
        <CoverButton id={node.id}>
          <KindIcon node={node} size={ICON_SMALL} />
          <span className={styles.title}>{node.title}</span>
        </CoverButton>
      </span>
      {ops.map(op => <NodeRef key={op.id} id={op.id} className={styles.op}>{opLine(op)}</NodeRef>)}
    </div>
  );
}

/**
 * An expanded container: a box with its head, holding what the view lays out inside it. The head previews the
 * container; the chevron collapses it when the view expanded it on request.
 */
export function ContainerBox({ node, style, collapsible, children }: { node: ViewNode; style?: CSSProperties; collapsible: boolean; children: ReactNode }) {
  const links = useLinks();
  const expandHref = collapsible ? links.expandHref?.(node.id) : undefined;
  return (
    <div className={styles.container} data-selected={links.selected === node.id} style={style}>
      <span className={styles.containerHead}>
        <StatusDot status={node.status} />
        {expandHref && <Toggle href={expandHref} open />}
        <CoverButton id={node.id}>
          <KindIcon node={node} size={ICON_SMALL} />
          <span className={styles.title}>{node.title}</span>
        </CoverButton>
        <NodeHint node={node} />
      </span>
      {children}
    </div>
  );
}
