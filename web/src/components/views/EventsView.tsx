import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import type { View, ViewNode } from '../../api/types';
import { ABOUT, EVENTS_COLUMNS, VIEW_NAMES } from '../../terms';
import { ICON_SMALL, ICON_TINY } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { CHIP_HEIGHT, chipWidth } from '../canvas/sizes';
import { Lines, type Line, type LineTone } from '../canvas/Edges';
import { route, type Rect } from '../canvas/layout';
import { monoWidth } from '../canvas/measure';
import { DiagnosticTag } from '../details/sections';
import { messageOf, relative, scopeOf, stepName } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useSelectedBox } from '../marks/links';
import { Above, CoverButton, NodeRef } from '../marks/NodeButton';
import { NodeChip } from '../marks/NodeChip';
import { StatusDot } from '../marks/StatusDot';
import styles from './views.module.css';

const COLUMN_GAP = 40;
const ROW_GAP = 28;
const HEAD = 28;
const SECTION_GAP = 44;
const CARD_PADDING = 24;
const LINE = 20;
const TAG = 28;

/** A card of the events view: a step or an operation, with a second line saying where it is or what it does. */
interface Card {
  node: ViewNode;
  second?: { node: ViewNode; text: string };
  tags: number;
}

/** One row: what publishes or raises, the event or error, what subscribes or handles. */
interface Row {
  left: Card[];
  middle: ViewNode;
  right: Card[];
}

function cardWidth(card: Card): number {
  return Math.max(chipWidth(stepName(card.node)), 2 * 8 + ICON_TINY + 4 + monoWidth(card.second?.text ?? '', 11)) + 8;
}

function cardHeight(card: Card): number {
  return CARD_PADDING + LINE + (card.second ? LINE - 2 : 0) + card.tags * TAG;
}

function EventCard({ card, diagnostics }: { card: Card; diagnostics: View['diagnostics'] }) {
  const { node } = card;
  const box = useSelectedBox(node.id);
  const label = node.kind === 'step' ? stepName(node) : relative(node.id, scopeOf(node.id));
  return (
    <div className={clsx(styles.eventCard, box.selected && 'loop-edge-still')} {...box.props}>
      <span className={styles.eventHead}>
        <CoverButton id={node.id}>
          <StatusDot status={node.status} />
          <KindIcon node={node} size={ICON_SMALL} />
          {label}
        </CoverButton>
      </span>
      {card.second && (
        <span className={styles.eventSecond}>
          <NodeRef id={card.second.node.id} className={styles.refLine}><KindIcon node={card.second.node} size={ICON_TINY} />{card.second.text}</NodeRef>
        </span>
      )}
      {diagnostics.filter(diagnostic => diagnostic.subject === node.id).map(diagnostic => <Above key={diagnostic.code}><DiagnosticTag diagnostic={diagnostic} /></Above>)}
    </div>
  );
}

/**
 * The events view (Figma 10): each event between the steps that publish it and the operations that subscribe to it,
 * and each error between the calls that raise it and the on fragments that handle it.
 */
export function EventsView({ view }: { view: View }) {
  const byId = new Map(view.nodes.map(node => [node.id, node]));
  const tagsOf = (id: string) => view.diagnostics.filter(diagnostic => diagnostic.subject === id).length;
  const targets = (from: string, kind: string) => view.edges.filter(edge => edge.from === from && edge.kind === kind).map(edge => edge.to);
  const sources = (to: string, kind: string) => view.edges.filter(edge => edge.to === to && edge.kind === kind).flatMap(edge => byId.get(edge.from) ?? []);
  const eventRows: Row[] = view.nodes.filter(node => node.kind === 'type' && node.form === 'event').map(event => ({
    middle: event,
    left: sources(event.id, 'emits').map(step => ({ node: step, second: step.step?.sender && { node: step.step.sender, text: step.step.sender.id }, tags: tagsOf(step.id) })),
    right: sources(event.id, 'subscribes').map(op => {
      const trigger = sources(op.id, 'calls').find(step => step.form === 'async');
      return { node: op, second: trigger && { node: trigger, text: stepName(trigger) }, tags: tagsOf(op.id) };
    }),
  }));
  const errorRows: Row[] = view.nodes.filter(node => node.kind === 'type' && node.form === 'error').map(error => {
    const raisingOps = new Set(view.edges.filter(edge => edge.kind === 'refers' && edge.to === error.id).map(edge => edge.from));
    const raisers = view.nodes.filter(node => node.kind === 'step' && targets(node.id, 'calls').some(op => raisingOps.has(op)));
    return {
      middle: error,
      left: raisers.map(step => ({ node: step, second: step.step?.target && { node: step.step.target, text: step.step.target.id }, tags: tagsOf(step.id) })),
      right: sources(error.id, 'handles').map(step => {
        const child = targets(step.id, 'contains').map(id => byId.get(id)).find(node => node !== undefined);
        return { node: step, second: child && { node: child, text: `${child.title} ${child.step?.receiver ? `${child.step.receiver.title}.` : ''}${messageOf(child)}` }, tags: tagsOf(step.id) };
      }),
    };
  });
  const all = [...eventRows, ...errorRows];
  const leftWidth = Math.max(160, ...all.flatMap(row => row.left.map(cardWidth)));
  const middleWidth = Math.max(140, ...all.map(row => chipWidth(row.middle.id) + 8));
  const rightWidth = Math.max(160, ...all.flatMap(row => row.right.map(cardWidth)));
  const columns = [0, leftWidth + COLUMN_GAP, leftWidth + middleWidth + 2 * COLUMN_GAP];
  const rects = new Map<string, Rect>();
  const lines: Line[] = [];
  const heads: { y: number; labels: string[] }[] = [];
  let y = 0;
  const placeRows = (rows: Row[], labels: string[]) => {
    if (rows.length === 0) {
      return;
    }
    heads.push({ y, labels });
    y += HEAD;
    for (const row of rows) {
      const stack = (cards: Card[], x: number, width: number) => {
        let top = y;
        return cards.map(card => {
          const rect = { x, y: top, width, height: cardHeight(card) };
          top += rect.height + 8;
          rects.set(`${row.middle.id} ${card.node.id}`, rect);
          return rect;
        });
      };
      const left = stack(row.left, columns[0] ?? 0, leftWidth);
      const right = stack(row.right, columns[2] ?? 0, rightWidth);
      const middle = { x: columns[1] ?? 0, y, width: chipWidth(row.middle.id) + 8, height: CHIP_HEIGHT };
      rects.set(row.middle.id, middle);
      const tone: LineTone = row.middle.form === 'event' ? 'event' : 'plain';
      left.forEach((rect, index) => lines.push({ id: `${row.middle.id} in ${index}`, route: route({ ...rect, height: Math.min(rect.height, CHIP_HEIGHT + 8) }, middle), tone }));
      right.forEach((rect, index) => lines.push({ id: `${row.middle.id} out ${index}`, route: route(middle, { ...rect, height: Math.min(rect.height, CHIP_HEIGHT + 8) }), tone: 'plain' }));
      y += Math.max(CHIP_HEIGHT, ...[...left, ...right].map(rect => rect.y + rect.height - y)) + ROW_GAP;
    }
    y += SECTION_GAP - ROW_GAP;
  };
  placeRows(eventRows, [EVENTS_COLUMNS.emitters, EVENTS_COLUMNS.events, EVENTS_COLUMNS.subscribers]);
  placeRows(errorRows, [EVENTS_COLUMNS.raisers, EVENTS_COLUMNS.errors, EVENTS_COLUMNS.handlers]);
  const width = (columns[2] ?? 0) + rightWidth;
  const height = y;
  const cards = (rows: Row[]): ReactNode[] => rows.flatMap(row => [...row.left, ...row.right].map(card => {
    const rect = rects.get(`${row.middle.id} ${card.node.id}`);
    return rect && <Placed key={`${row.middle.id} ${card.node.id}`} rect={rect}><EventCard card={card} diagnostics={view.diagnostics} /></Placed>;
  }));
  return (
    <Canvas about={ABOUT.events} label={VIEW_NAMES.events} width={width} height={height}>
      {heads.map(head => head.labels.map((label, index) => (
        <Placed key={`${head.y} ${label}`} rect={{ x: columns[index] ?? 0, y: head.y, width: 120, height: HEAD }} className={styles.columnHead}>{label}</Placed>
      )))}
      <Lines width={width} height={height} lines={lines} />
      {all.map(row => {
        const rect = rects.get(row.middle.id);
        return rect && (
          <Placed key={row.middle.id} rect={rect} className={styles.chipSlot}>
            <NodeChip node={row.middle} label={row.middle.id} className={row.middle.form === 'event' ? styles.eventPill : undefined} />
          </Placed>
        );
      })}
      {cards(all)}
    </Canvas>
  );
}
