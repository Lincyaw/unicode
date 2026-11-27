import { clsx } from 'clsx';
import { ChevronDown, ChevronRight } from '../marks/icons';
import type { CSSProperties, ReactNode } from 'react';
import { Link } from 'react-router';
import type { Node, ViewNode } from '../../api/types';
import { ABOUT, CONTROLS, kindWord } from '../../terms';
import { Hint, InfoHint } from '../../ui/hint';
import { opLine } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks, useSelectedBox } from '../marks/links';
import { Above } from '../marks/Above';
import { CoverButton } from '../marks/CoverButton';
import { NodeRef } from '../marks/NodeRef';
import { TypeText } from '../marks/Signature';
import { StatusDot } from '../marks/StatusDot';
import { useReveal } from './panZoom';
import type { CardRow } from './sizes';
import styles from './cards.module.css';
import { Icon } from '../../ui/icons';

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
      <InfoHint subject={node.id}>
        <span>{ABOUT.card(kindWord(node.kind, node.form, node.layer), node.form, `${node.origin.path}:${node.origin.line}`)}</span>
        {node.summary !== '' && <span>{node.summary}</span>}
      </InfoHint>
    </Above>
  );
}

function Rows({ rows, onUnfold }: { rows: CardRow[]; onUnfold?: () => void }) {
  return rows.map((row, index) => {
    switch (row.kind) {
      case 'field':
        return (
          <span key={index} className={styles.field} data-warn={row.warn}>
            <span>{row.name}</span>
            <span className={styles.fieldType}><TypeText parts={row.type} references="preview" /></span>
          </span>
        );
      case 'value':
        return <span key={index} className={styles.field} data-warn={row.warn}><span>{row.name}</span></span>;
      case 'op':
        // A long signature is cut short on the card; its whole text shows while the pointer rests on it.
        return <Hint key={index} content={row.text}><NodeRef id={row.op.id} className={styles.op}>{row.text}</NodeRef></Hint>;
      case 'more':
        return <button key={index} type="button" className={clsx(styles.field, styles.more)} aria-expanded="false" onClick={onUnfold}>+{row.count}</button>;
    }
  });
}

/**
 * A design type drawn as a card: its fields or values and its operations; a contradicted type has a red edge. Its +n
 * unfolds the card to all its fields in place (`onUnfold`), and the canvas keeps the card in sight as the drawing
 * makes room.
 */
export function TypeCard({ node, rows, onUnfold, style }: { node: ViewNode; rows: CardRow[]; onUnfold?: () => void; style?: CSSProperties }) {
  const box = useSelectedBox(node.id);
  const reveal = useReveal();
  const unfold = onUnfold && (() => {
    onUnfold();
    reveal(node.id);
  });
  return (
    <div className={clsx(styles.card, box.selected && 'loop-edge-still')} data-status={node.status} style={style} {...box.props}>
      <span className={styles.head}>
        <CoverButton id={node.id}>
          <StatusDot status={node.status} />
          <KindIcon node={node} size="small" />
          <span className={styles.title}>{node.title}</span>
        </CoverButton>
        <NodeHint node={node} />
      </span>
      <Rows rows={rows} onUnfold={unfold} />
    </div>
  );
}

/** The chevron that expands or collapses a container in place, lifted over the box's cover. */
function Toggle({ href, open }: { href: string; open: boolean }) {
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <Above>
      <Link className={styles.toggle} to={href} aria-label={open ? CONTROLS.collapseRow : CONTROLS.expandRow}>
        <Icon glyph={Chevron} size="tiny" />
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
  const box = useSelectedBox(node.id);
  const expandHref = links.expandHref?.(node.id);
  return (
    <div className={clsx(styles.card, styles.folded, box.selected && 'loop-edge-still')} style={style} {...box.props}>
      <span className={styles.head}>
        <StatusDot status={node.status} />
        {expandHref && <Toggle href={expandHref} open={false} />}
        <CoverButton id={node.id}>
          <KindIcon node={node} size="small" />
          <span className={clsx(styles.title, styles.mutedTitle)}>{node.title}</span>
        </CoverButton>
      </span>
      {names.length > 0 && (
        <span className={styles.names}>
          {names.map(child => (
            <NodeRef key={child.id} id={child.id} className={styles.name}>
              <KindIcon node={child} size="tiny" />
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
  const box = useSelectedBox(node.id);
  const ops = (node.contents ?? []).filter(child => child.kind === 'op');
  return (
    <div className={clsx(styles.card, styles.external, box.selected && 'loop-edge-still')} style={style} {...box.props}>
      <span className={styles.head}>
        <CoverButton id={node.id}>
          <KindIcon node={node} size="small" />
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
  const box = useSelectedBox(node.id);
  const expandHref = collapsible ? links.expandHref?.(node.id) : undefined;
  return (
    <div className={clsx(styles.container, box.selected && 'loop-edge-still')} style={style} {...box.props}>
      <span className={styles.containerHead}>
        <StatusDot status={node.status} />
        {expandHref && <Toggle href={expandHref} open />}
        <CoverButton id={node.id}>
          <KindIcon node={node} size="small" />
          <span className={styles.title}>{node.title}</span>
        </CoverButton>
        <NodeHint node={node} />
      </span>
      {children}
    </div>
  );
}
