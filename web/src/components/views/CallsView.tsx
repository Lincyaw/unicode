import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ABOUT, SECTIONS, VIEW_NAMES } from '../../terms';
import { ButtonLink } from '../../ui/controls';
import { Icon } from '../../ui/icons';
import { Canvas, Placed } from '../canvas/Canvas';
import { portNear } from '../canvas/channels';
import { LineMarks, Lines, type Line, type LineMark } from '../canvas/Edges';
import { layOut, type LayeredLink, type Stack } from '../canvas/layered';
import { routeThrough, type Rect, type Route } from '../canvas/layout';
import { CHIP_HEIGHT, chipWidth } from '../canvas/sizes';
import { statusOf } from '../marks/format';
import { ChevronUp } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { CallMark } from './CallMark';
import { edgeKey, markOf } from './graphParts';
import styles from './views.module.css';

const SPACING = { node: 28, rank: 72 };

/** How far apart the lines from the functions realizing a design operation run up to it, in the gutter left of those functions. */
const GUTTER_STEP = 8;

/** The width of the button that collapses the view's expansions, under the drawing. */
const COLLAPSE_WIDTH = 120;

/** How a function of the calls view is drawn: salient, a detail shown on request, or a folded component. */
function stateOf(node: ViewNode, folded: ReadonlySet<string>): 'default' | 'detail' | 'folded' {
  if (folded.has(node.id)) {
    return 'folded';
  }
  return node.kind === 'op' && !node.salient ? 'detail' : 'default';
}

function toneOf(edge: ViewEdge, details: ReadonlySet<string>): Line['tone'] {
  if (edge.kind === 'reaches') {
    return 'reach';
  }
  return details.has(edge.from) || details.has(edge.to) ? 'faint' : 'plain';
}

/** Where the view says a node stands (CallSide); the calls view gives every node its side. */
function sideOf(node: ViewNode | undefined): NonNullable<ViewNode['side']> {
  if (node?.side === undefined) {
    throw new Error(`the calls view gives ${node?.id ?? 'a node'} no side`);
  }
  return node.side;
}

/**
 * The calls view (Figma 04): the roots, which are the input and the functions realizing it, in the middle; the functions
 * calling a root on their left; the salient functions the roots call or reach on their right, column by column. Each
 * node stands on the side the view gives it (`side`); the page derives none. A design operation stands over the functions realizing
 * it, joined to each by its realizes line. A reaches edge, dashed, counts the detail functions it passes; functions of
 * other components merge into their folded component, the line counting the calls it stands for; an expanded function
 * shows the detail functions it calls. The lines run as layered.ts lays them, each on a track of its own, with their
 * marks in columns of their own.
 */
export function CallsView({ view }: { view: View }) {
  const links = useLinks();
  const folded = new Set(view.folded);
  const details = new Set(view.nodes.filter(node => stateOf(node, folded) === 'detail').map(node => node.id));
  const input = view.input.id;
  const nodes = new Map(view.nodes.map(node => [node.id, node]));
  const bindings = view.edges.filter(edge => edge.kind === 'realizes' && edge.to === input);
  const realizers = bindings.map(edge => edge.from);
  const calls = view.edges.filter(edge => edge.kind !== 'realizes');
  const callee = (id: string) => sideOf(nodes.get(id)) === 'callee';
  const stacks: Stack[] = realizers.length > 0
    ? [{ id: `stack ${input}`, members: [{ id: input, indent: 0 }, ...realizers.map(id => ({ id, indent: GUTTER_STEP * (realizers.length + 1) }))] }]
    : [];
  // A call from a callee back to a root or to a caller ranks the other way, so that callers stay left and callees right.
  const callLinks: LayeredLink[] = calls.map(edge => {
    const mark = markOf(edge);
    return { id: edgeKey(edge), from: edge.from, to: edge.to, reversed: callee(edge.from) && !callee(edge.to), ...(mark ? { mark } : {}) };
  });
  const layout = layOut(view.nodes.map(node => ({ id: node.id, width: chipWidth(node.title, stateOf(node, folded) !== 'detail'), height: CHIP_HEIGHT })), callLinks, 'LR', SPACING, { stacks });
  const rectOf = (id: string): Rect => {
    const rect = layout.rects.get(id);
    if (!rect) {
      throw new Error(`the calls view placed no box for ${id}`);
    }
    return rect;
  };
  const routeOf = (edge: ViewEdge): Route => {
    const route = layout.routes.get(edgeKey(edge));
    if (!route) {
      throw new Error(`the calls view laid out no line for ${edgeKey(edge)}`);
    }
    return route;
  };
  // Each realizing function's line leaves its left side, runs into the gutter on a step of its own and up to the operation.
  const bindingLines: Line[] = bindings.map((edge, index) => {
    const op = rectOf(input);
    const fn = rectOf(edge.from);
    const x = op.x + GUTTER_STEP * (index + 1);
    const y = portNear(fn.y + fn.height / 2, 'start');
    return { id: edgeKey(edge), route: routeThrough([{ x: fn.x, y }, { x, y }, { x, y: op.y + op.height }]), tone: 'plain' };
  });
  const lines: Line[] = [
    ...calls.map(edge => ({ id: edgeKey(edge), route: routeOf(edge), tone: toneOf(edge, details), select: edge.kind === 'reaches' ? edgeKey(edge) : undefined })),
    ...bindingLines,
  ];
  const marks: LineMark[] = calls.filter(edge => markOf(edge) !== undefined).map(edge => ({ id: edgeKey(edge), route: routeOf(edge), sides: ['center'], children: <CallMark edge={edge} /> }));
  const collapse = view.expand.length > 0 ? links.expandHref?.(view.expand[0] ?? '') : undefined;
  const height = layout.height + (collapse ? 2 * CHIP_HEIGHT : 0);
  return (
    <Canvas about={ABOUT.callsView} label={VIEW_NAMES.calls} width={layout.width} height={height}>
      <Lines width={layout.width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {view.nodes.map(node => (
        <Placed key={node.id} rect={rectOf(node.id)} className={styles.chipSlot}>
          <NodeChip node={node} state={stateOf(node, folded)} status={statusOf(node)} />
        </Placed>
      ))}
      <LineMarks marks={marks} obstacles={view.nodes.map(node => rectOf(node.id))} />
      {collapse && (
        <Placed rect={{ x: 0, y: layout.height + CHIP_HEIGHT, width: COLLAPSE_WIDTH, height: CHIP_HEIGHT }} className={styles.chipSlot}>
          <ButtonLink to={collapse} variant="line" size="small"><Icon glyph={ChevronUp} size="tiny" />{SECTIONS.collapse}</ButtonLink>
        </Placed>
      )}
    </Canvas>
  );
}
