import type { KeyboardEvent } from 'react';
import type { View, ViewNode } from '../../api/types';
import { ABOUT, VIEW_NAMES } from '../../terms';
import { ICON_SMALL, ICON_TINY } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { Lines, type Line } from '../canvas/Edges';
import { monoWidth } from '../canvas/measure';
import { chipWidth } from '../canvas/sizes';
import { messageOf, relative, scopeOf } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeRef } from '../marks/NodeButton';
import { StatusDot } from '../marks/StatusDot';
import { StepLabel } from './StepLabel';
import { descendants, diagnosticsAbout, stepTree, type StepTree } from './stepTree';
import styles from './views.module.css';

/*
 * The geometry of a sequence: lane heads in a row, then one band per step down the lanes. A message's arrow runs
 * LABEL below its band's top, under its label; an emitted event takes a pill and a label; a fragment boxes its steps.
 */
const HEAD_HEIGHT = 30;
const LANE_GAP = 6;
const FIRST_BAND = 57;
const BAND = 48;
const LABEL = 22;
const EMIT_BAND = 50;
const EMIT_PILL = 16;
const EMIT_LABEL = 36;
const ON_HEAD = 29;
const ASYNC_HEAD = 45;
const FRAGMENT_FOOT = 6;
const FRAGMENT_GAP = 40;
const ACTIVATION = 10;
const FRAGMENT_LEFT = 36;
const FRAGMENT_RIGHT = 66;
const PILL_OFFSET = 56;
const MARGIN = { top: 24, side: 12 };

interface Lane {
  node: ViewNode;
  x: number;
  width: number;
}

/** What the drawing holds for each step: a message, an emitted event, or a fragment's box. */
type Mark =
  | { kind: 'message'; step: ViewNode; y: number; from: number; to: number }
  | { kind: 'emit'; step: ViewNode; y: number; from: number; pill: number }
  | { kind: 'fragment'; step: ViewNode; top: number; bottom: number; left: number; right: number };

interface Activation {
  x: number;
  top: number;
  bottom: number;
}

function laneWidth(node: ViewNode): number {
  if (node.kind === 'actor') {
    return node.form === 'person' ? ICON_SMALL + 8 + monoWidth(node.title, 12) : chipWidth(node.title, false);
  }
  return chipWidth(node.title);
}

/** Lays the steps out down the lanes, fragments boxing their steps and calls with sub-steps activating their receiver. */
function layout(tree: StepTree) {
  let x = 0;
  const lanes: Lane[] = tree.lifelines.map(node => {
    const width = laneWidth(node);
    const lane = { node, x: x + width / 2, width };
    x += width + LANE_GAP;
    return lane;
  });
  const laneX = new Map(lanes.map(lane => [lane.node.id, lane.x]));
  const marks: Mark[] = [];
  const activations: Activation[] = [];
  const active = new Set<string>();
  /** Where an arrow leaves or reaches a lane: its lifeline, or the side of its activation that faces the other end. */
  const side = (id: string | undefined, toward: number, activated: boolean) => {
    const at = laneX.get(id ?? '') ?? 0;
    return activated ? at + (toward >= at ? ACTIVATION / 2 : -ACTIVATION / 2) : at;
  };
  let cursor = FIRST_BAND;
  const walk = (step: ViewNode): number => {
    const info = step.step;
    const children = tree.children(step);
    if (step.form === 'on' || step.form === 'async') {
      const involved = [step, ...descendants(tree, step)].flatMap(node => [node.step?.sender?.id, node.step?.receiver?.id]).flatMap(id => (id && laneX.has(id) ? [laneX.get(id) ?? 0] : []));
      const top = cursor;
      cursor += step.form === 'async' ? ASYNC_HEAD : ON_HEAD;
      const receiver = step.form === 'async' ? info?.receiver?.id : undefined;
      if (receiver) {
        active.add(receiver);
      }
      const last = children.map(walk).at(-1) ?? cursor;
      if (receiver) {
        active.delete(receiver);
        activations.push({ x: laneX.get(receiver) ?? 0, top: top + ASYNC_HEAD - 9, bottom: last + LABEL - 2 });
      }
      const bottom = cursor + FRAGMENT_FOOT;
      marks.push({ kind: 'fragment', step, top, bottom, left: Math.min(...involved) - FRAGMENT_LEFT, right: Math.max(...involved) + FRAGMENT_RIGHT });
      cursor = bottom + FRAGMENT_GAP;
      return bottom;
    }
    if (step.form === 'emit') {
      const at = laneX.get(info?.sender?.id ?? '') ?? 0;
      const y = cursor + EMIT_PILL;
      marks.push({ kind: 'emit', step, y, from: side(info?.sender?.id, at + 1, active.has(info?.sender?.id ?? '')), pill: at + PILL_OFFSET });
      cursor += EMIT_BAND;
      return y;
    }
    const receiver = info?.receiver?.id ?? info?.target?.id;
    const target = laneX.get(receiver ?? '') ?? 0;
    const from = side(info?.sender?.id, target, active.has(info?.sender?.id ?? ''));
    const y = cursor + LABEL;
    const activates = children.length > 0 && receiver !== undefined && !active.has(receiver);
    marks.push({ kind: 'message', step, y, from, to: side(receiver, from, activates || active.has(receiver ?? '')) });
    cursor += BAND;
    if (children.length > 0 && receiver !== undefined) {
      if (activates) {
        active.add(receiver);
      }
      const last = children.map(walk).at(-1) ?? y;
      if (activates) {
        active.delete(receiver);
        activations.push({ x: target, top: y - 7, bottom: last + 5 });
      }
    }
    return y;
  };
  tree.roots.forEach(walk);
  const width = Math.max(x - LANE_GAP, ...marks.flatMap(mark => (mark.kind === 'fragment' ? [mark.right] : mark.kind === 'emit' ? [mark.pill + 140] : [])));
  return { lanes, marks, activations, height: cursor, width };
}

function LaneHead({ lane }: { lane: Lane }) {
  const links = useLinks();
  const { node } = lane;
  return (
    <div
      className={styles.laneHead} data-person={node.kind === 'actor' && node.form === 'person'} data-external={node.kind === 'actor' && node.form === 'external'}
      aria-pressed={links.selected === node.id} role="button" tabIndex={0} onClick={() => links.select(node.id)} onDoubleClick={() => links.open(node.id)}
      onKeyDown={(event: KeyboardEvent) => (event.key === 'Enter' ? links.open(node.id) : undefined)}
    >
      {node.kind !== 'actor' && <StatusDot status={node.status} />}
      <KindIcon node={node} size={ICON_SMALL} />
      {node.title}
    </div>
  );
}

/**
 * The sequence view (Figma 02): the flow's lifelines in the order they first appear, each step a message between them
 * with its index and status, nested activations, events published as pills, and on and async fragments as boxes.
 */
export function SequenceView({ view }: { view: View }) {
  const links = useLinks();
  const tree = stepTree(view);
  const { lanes, marks, activations, height, width } = layout(tree);
  const scope = scopeOf(view.input.id);
  const lines: Line[] = [
    ...lanes.map((lane): Line => ({ id: `lane ${lane.node.id}`, route: { d: `M ${lane.x} ${HEAD_HEIGHT} V ${height}`, mid: { x: lane.x, y: 0 } }, tone: 'faint', plain: true })),
    ...marks.flatMap((mark): Line[] => {
      switch (mark.kind) {
        case 'message':
          return [{ id: mark.step.id, route: { d: `M ${mark.from} ${mark.y} H ${mark.to}`, mid: { x: (mark.from + mark.to) / 2, y: mark.y } }, tone: links.selected === mark.step.id ? 'brandSolid' : 'plain' }];
        case 'emit':
          return [{ id: mark.step.id, route: { d: `M ${mark.from} ${mark.y} H ${mark.pill}`, mid: { x: mark.pill, y: mark.y } }, tone: 'event' }];
        case 'fragment':
          return [];
      }
    }),
  ];
  const fragments = marks.flatMap(mark => (mark.kind === 'fragment' ? [mark] : []));
  return (
    <Canvas about={ABOUT.sequence} label={VIEW_NAMES.sequence} width={width} height={height} margin={MARGIN} aboutBelow>
      {fragments.map(mark => (
        <Placed key={mark.step.id} rect={{ x: mark.left, y: mark.top, width: mark.right - mark.left, height: mark.bottom - mark.top }} className={styles.fragment}>
          <span className={styles.fragmentIndex}><NodeRef id={mark.step.id}>{mark.step.title}</NodeRef></span>
        </Placed>
      ))}
      <Lines width={width} height={height} lines={lines} />
      {activations.map(activation => (
        <Placed key={`${activation.x} ${activation.top}`} rect={{ x: activation.x - ACTIVATION / 2, y: activation.top, width: ACTIVATION, height: activation.bottom - activation.top }} className={styles.activation}>{null}</Placed>
      ))}
      {lanes.map(lane => <Placed key={lane.node.id} rect={{ x: lane.x - lane.width / 2, y: 0, width: lane.width, height: HEAD_HEIGHT }}><LaneHead lane={lane} /></Placed>)}
      {marks.map(mark => {
        switch (mark.kind) {
          case 'message':
            return (
              <Placed key={mark.step.id} rect={{ x: Math.min(mark.from, mark.to) + 6, y: mark.y - LABEL, width: 0, height: LABEL - 2 }} className={styles.messageSlot}>
                <StepLabel step={mark.step} diagnostics={diagnosticsAbout(view, mark.step.id)}>{messageOf(mark.step)}</StepLabel>
              </Placed>
            );
          case 'emit':
            return (
              <div key={mark.step.id}>
                <Placed rect={{ x: mark.pill, y: mark.y - 12, width: 0, height: 24 }} className={styles.messageSlot}>
                  {mark.step.step?.target && (
                    <NodeRef id={mark.step.step.target.id} className={styles.pill}>
                      <KindIcon node={mark.step} size={ICON_TINY} />
                      {relative(mark.step.step.target.id, scope)}
                    </NodeRef>
                  )}
                </Placed>
                <Placed rect={{ x: mark.from + 6, y: mark.y + EMIT_LABEL - EMIT_PILL - LABEL / 2, width: 0, height: LABEL - 2 }} className={styles.messageSlot}>
                  <StepLabel step={mark.step} diagnostics={diagnosticsAbout(view, mark.step.id)}>emits {messageOf(mark.step)}</StepLabel>
                </Placed>
              </div>
            );
          case 'fragment':
            return null;
        }
      })}
      {fragments.map(mark => (
        <Placed key={`tag ${mark.step.id}`} rect={{ x: mark.left + 8, y: mark.top + 8, width: 0, height: LABEL }} className={styles.messageSlot}>
          <StepLabel step={mark.step} diagnostics={diagnosticsAbout(view, mark.step.id)} className={styles.fragmentTag}>
            <KindIcon node={mark.step} size={ICON_TINY} />
            {mark.step.form} {relative(mark.step.step?.target?.id ?? '', scope)}
          </StepLabel>
        </Placed>
      ))}
    </Canvas>
  );
}
