import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import type { View, ViewNode } from '../../api/types';
import { ABOUT, VIEW_NAMES } from '../../terms';
import { ICON_TINY } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { Lines, type Line, type LineTone } from '../canvas/Edges';
import { route, type Rect } from '../canvas/layout';
import { monoWidth } from '../canvas/measure';
import { messageOf, relative, scopeOf } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { StepLabel } from './StepLabel';
import { diagnosticsAbout, stepTree, type StepTree } from './stepTree';
import styles from './views.module.css';

const BOX_HEIGHT = 30;
const GAP = 30;
const GROUP_PADDING = 12;
const GROUP_HEAD = 34;
const DOT = 16;
const DIAMOND = 24;
const BRANCH_GAP = 180;
const ASYNC_GAP = 56;
const ASYNC_PADDING = 22;
const ASYNC_HEAD = 46;

/** What the activity drawing places: a step's box, a call's group of boxes, the start, an end, a decision or an event. */
type Piece =
  | { kind: 'step'; step: ViewNode; rect: Rect }
  | { kind: 'group'; step: ViewNode; rect: Rect }
  | { kind: 'start' | 'end'; rect: Rect }
  | { kind: 'decision'; step: ViewNode; rect: Rect; tag: Rect }
  | { kind: 'event'; step: ViewNode; rect: Rect }
  | { kind: 'async'; step: ViewNode; rect: Rect };

/** What a step's box says: what it calls, on whom, or the event it emits. */
function boxText(step: ViewNode): string {
  if (step.form === 'emit') {
    return `emits ${messageOf(step)}`;
  }
  const receiver = step.step?.receiver;
  return receiver ? `${receiver.title}.${messageOf(step)}` : messageOf(step);
}

function boxWidth(step: ViewNode): number {
  return monoWidth(`${step.title}  ${boxText(step)}`, 12) + 2 * 12 + 20;
}

interface Drawn {
  pieces: Piece[];
  arrows: { from: Rect; to: Rect; tone?: LineTone }[];
}

/** A run of steps one under another from `top`, centred on `x`; it returns where it ends. */
function column(tree: StepTree, steps: ViewNode[], x: number, top: number, drawn: Drawn, previous?: Rect): { bottom: number; last?: Rect; width: number } {
  let y = top;
  let last = previous;
  let width = 0;
  for (const step of steps) {
    const children = tree.children(step);
    if (step.form === 'on') {
      const diamond = { x: x - DIAMOND / 2, y, width: DIAMOND, height: DIAMOND };
      if (last) {
        drawn.arrows.push({ from: last, to: diamond });
      }
      const branch = column(tree, children, x + BRANCH_GAP + 60, y - (BOX_HEIGHT - DIAMOND) / 2, drawn);
      const first = drawn.pieces.find(piece => piece.kind === 'step' && piece.step.id === children[0]?.id);
      if (first) {
        drawn.arrows.push({ from: diamond, to: first.rect });
      }
      const label = { x: x + DIAMOND, y: y - BOX_HEIGHT + 4, width: BRANCH_GAP, height: 24 };
      drawn.pieces.push({ kind: 'decision', step, rect: diamond, tag: label });
      last = diamond;
      width = Math.max(width, BRANCH_GAP + 60 + branch.width);
      y = Math.max(y + DIAMOND, branch.bottom) + GAP;
      if (branch.last) {
        drawn.pieces.push({ kind: 'end', rect: { x: x - DOT / 2, y, width: DOT, height: DOT } });
        drawn.arrows.push({ from: diamond, to: { x: x - DOT / 2, y, width: DOT, height: DOT } });
        drawn.arrows.push({ from: branch.last, to: { x: x - DOT / 2, y, width: DOT, height: DOT } });
        return { bottom: y + DOT, width, last: undefined };
      }
      continue;
    }
    const nested = children.length > 0 && step.step?.parent !== undefined;
    if (nested) {
      const inner: Drawn = { pieces: [], arrows: [] };
      const content = column(tree, children, x, y + GROUP_HEAD, inner);
      const groupWidth = Math.max(boxWidth(step), content.width) + 2 * GROUP_PADDING;
      const rect = { x: x - groupWidth / 2, y, width: groupWidth, height: content.bottom - y + GROUP_PADDING - GAP };
      drawn.pieces.push({ kind: 'group', step, rect }, ...inner.pieces);
      drawn.arrows.push(...inner.arrows);
      if (last) {
        drawn.arrows.push({ from: last, to: rect });
      }
      last = rect;
      width = Math.max(width, groupWidth);
      y = rect.y + rect.height + GAP;
      continue;
    }
    const rect = { x: x - boxWidth(step) / 2, y, width: boxWidth(step), height: BOX_HEIGHT };
    drawn.pieces.push({ kind: 'step', step, rect });
    if (last) {
      drawn.arrows.push({ from: last, to: rect });
    }
    last = rect;
    width = Math.max(width, rect.width);
    y += BOX_HEIGHT + GAP;
    // The flow's first call is its trigger: what it does follows on the main line.
    if (children.length > 0) {
      const rest = column(tree, children, x, y, drawn, last);
      return { bottom: rest.bottom, last: rest.last, width: Math.max(width, rest.width) };
    }
  }
  return { bottom: y, last, width };
}

/** The step and the event that trigger an async fragment: the event its operation subscribes to, and the step that emits it. */
function triggerOf(view: View, step: ViewNode): { event: string; emitter?: ViewNode } | undefined {
  const op = step.step?.target?.id;
  const event = view.edges.find(edge => edge.kind === 'subscribes' && edge.from === op)?.to;
  if (event === undefined) {
    return undefined;
  }
  return { event, emitter: view.nodes.find(node => node.form === 'emit' && node.step?.target?.id === event) };
}

/**
 * The activity view (Figma 09): the same flow by its control structure. The main line runs from the start through the
 * trigger's steps; a call with sub-steps is a box around them; an on fragment is a decision whose branch rejoins at the
 * end; an async fragment is a dashed box beside, entered from the event that triggers it.
 */
export function ActivityView({ view }: { view: View }) {
  const links = useLinks();
  const tree = stepTree(view);
  const drawn: Drawn = { pieces: [], arrows: [] };
  const main = tree.roots.filter(step => step.form !== 'async');
  const asyncs = tree.roots.filter(step => step.form === 'async');
  const probe: Drawn = { pieces: [], arrows: [] };
  const mainWidth = column(tree, main, 0, 0, probe).width;
  const x = mainWidth / 2 + 8;
  const start = { x: x - DOT / 2, y: 0, width: DOT, height: DOT };
  drawn.pieces.push({ kind: 'start', rect: start });
  const body = column(tree, main, x, DOT + GAP, drawn, start);
  if (body.last) {
    const end = { x: x - DOT / 2, y: body.bottom, width: DOT, height: DOT };
    drawn.pieces.push({ kind: 'end', rect: end });
    drawn.arrows.push({ from: body.last, to: end });
  }
  const scope = scopeOf(view.input.id);
  const mainPieces = [...drawn.pieces];
  let asyncTop = 0;
  for (const step of asyncs) {
    const trigger = triggerOf(view, step);
    const inner: Drawn = { pieces: [], arrows: [] };
    const sized = column(tree, tree.children(step), 0, 0, { pieces: [], arrows: [] });
    const innerWidth = Math.max(sized.width, 200);
    const boxHeight = ASYNC_HEAD + (trigger ? BOX_HEIGHT + GAP : 0) + sized.bottom + DOT + ASYNC_PADDING;
    // The box stands right of whatever the main line draws beside it, not of the whole drawing.
    const beside = mainPieces.filter(piece => piece.rect.y < asyncTop + boxHeight && piece.rect.y + piece.rect.height > asyncTop);
    const left = Math.max(0, ...beside.map(piece => piece.rect.x + piece.rect.width)) + ASYNC_GAP;
    const cx = left + ASYNC_PADDING + innerWidth / 2;
    let y = asyncTop + ASYNC_HEAD;
    let previous: Rect | undefined;
    if (trigger) {
      const pill = { x: cx - (monoWidth(relative(trigger.event, scope), 12) + 40) / 2, y, width: monoWidth(relative(trigger.event, scope), 12) + 40, height: BOX_HEIGHT };
      inner.pieces.push({ kind: 'event', step: trigger.emitter ?? step, rect: pill });
      const emitter = trigger.emitter && drawn.pieces.find(piece => piece.kind === 'step' && piece.step.id === trigger.emitter?.id);
      if (emitter) {
        drawn.arrows.push({ from: emitter.rect, to: pill, tone: 'event' });
      }
      previous = pill;
      y += BOX_HEIGHT + GAP;
    }
    const content = column(tree, tree.children(step), cx, y, inner, previous);
    const end = { x: cx - DOT / 2, y: content.bottom, width: DOT, height: DOT };
    if (content.last) {
      inner.pieces.push({ kind: 'end', rect: end });
      inner.arrows.push({ from: content.last, to: end });
    }
    const box = { x: left, y: asyncTop, width: innerWidth + 2 * ASYNC_PADDING, height: end.y + DOT + ASYNC_PADDING - asyncTop };
    drawn.pieces.push({ kind: 'async', step, rect: box }, ...inner.pieces);
    drawn.arrows.push(...inner.arrows);
    asyncTop = box.y + box.height + GAP;
  }
  const width = Math.max(...drawn.pieces.map(piece => piece.rect.x + piece.rect.width));
  const height = Math.max(...drawn.pieces.map(piece => piece.rect.y + piece.rect.height));
  const lines: Line[] = drawn.arrows.map((arrow, index) => ({ id: String(index), route: route(arrow.from, arrow.to), tone: arrow.tone ?? 'plain' }));
  const label = (step: ViewNode, children: ReactNode, className?: string) => <StepLabel step={step} diagnostics={diagnosticsAbout(view, step.id)} className={className}>{children}</StepLabel>;
  return (
    <Canvas about={ABOUT.activity} label={VIEW_NAMES.activity} width={width} height={height}>
      {drawn.pieces.filter(piece => piece.kind === 'async' || piece.kind === 'group').map(piece => 'step' in piece && (
        <Placed key={`box ${piece.step.id}`} rect={piece.rect} className={clsx(styles.activityBox, piece.kind === 'async' && styles.asyncBox)}>
          {piece.kind === 'async'
            ? <span className={styles.asyncHead}><span className={styles.fragmentIndex}>{piece.step.title}</span>{label(piece.step, <><KindIcon node={piece.step} size={ICON_TINY} />async {relative(piece.step.step?.target?.id ?? '', scope)}</>, styles.fragmentTag)}</span>
            : <span className={styles.groupHead}>{label(piece.step, boxText(piece.step), styles.plainLabel)}</span>}
        </Placed>
      ))}
      <Lines width={width} height={height} lines={lines} />
      {drawn.pieces.map(piece => {
        switch (piece.kind) {
          case 'start':
            return <Placed key={`start ${piece.rect.y}`} rect={piece.rect} className={styles.startDot}>{null}</Placed>;
          case 'end':
            return <Placed key={`end ${piece.rect.x} ${piece.rect.y}`} rect={piece.rect} className={styles.endDot}>{null}</Placed>;
          case 'step':
            return (
              <Placed key={piece.step.id} rect={piece.rect} className={styles.activityStep}>
                {label(piece.step, <>{piece.step.form === 'emit' && <KindIcon node={piece.step} size={ICON_TINY} />}{boxText(piece.step)}</>, styles.boxLabel)}
              </Placed>
            );
          case 'decision':
            return (
              <div key={piece.step.id}>
                <Placed rect={piece.rect} className={styles.diamondSlot}>
                  <button type="button" className={styles.diamond} aria-pressed={links.selected === piece.step.id} aria-label={piece.step.id} onClick={() => links.select(piece.step.id)} onDoubleClick={() => links.open(piece.step.id)} />
                </Placed>
                <Placed rect={{ x: piece.rect.x - 40, y: piece.rect.y, width: 32, height: DIAMOND }} className={styles.decisionIndex}>{piece.step.title}</Placed>
                <Placed rect={piece.tag} className={styles.messageSlot}>
                  {label(piece.step, <><KindIcon node={piece.step} size={ICON_TINY} />on {relative(piece.step.step?.target?.id ?? '', scope)}</>, styles.fragmentTag)}
                </Placed>
              </div>
            );
          case 'event':
            return (
              <Placed key={`event ${piece.step.id}`} rect={piece.rect} className={styles.chipSlot}>
                <span className={styles.pill}><KindIcon node={{ kind: 'type', form: 'event', layer: 'design', salient: true }} size={ICON_TINY} />{piece.step.step?.target ? relative(piece.step.step.target.id, scope) : ''}</span>
              </Placed>
            );
          case 'group':
          case 'async':
            return null;
        }
      })}
    </Canvas>
  );
}
