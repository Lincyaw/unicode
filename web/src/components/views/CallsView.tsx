import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ABOUT, MARKS, SECTIONS, VIEW_NAMES } from '../../terms';
import { Tag } from '../../ui/content';
import { ButtonLink } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { CHIP_HEIGHT, chipWidth } from '../canvas/sizes';
import { Lines, Marker, type Line } from '../canvas/Edges';
import { arrange, route, type Link as LayoutLink, type Rect, type Route } from '../canvas/layout';
import { statusOf } from '../marks/format';
import { ViaTag } from '../details/sections';
import { ChevronUp, Merge } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { edgeKey } from './graphParts';
import styles from './views.module.css';
import { Icon } from '../../ui/icons';

const SPACING = { node: 28, rank: 160 };
const DETAIL_GAP = 72;
const DETAIL_DROP = 96;
const BUS_ABOVE = 28;

/** How a function of the calls view is drawn: salient, a detail shown on request, or a folded component. */
function stateOf(node: ViewNode, folded: ReadonlySet<string>): 'default' | 'detail' | 'folded' {
  if (folded.has(node.id)) {
    return 'folded';
  }
  return node.kind === 'op' && !node.salient ? 'detail' : 'default';
}

function lineOf(edge: ViewEdge, details: ReadonlySet<string>): Line['tone'] {
  if (edge.kind === 'reaches') {
    return 'reach';
  }
  return details.has(edge.from) || details.has(edge.to) ? 'faint' : 'plain';
}

/**
 * A line to or from a detail function in the row under the drawing: down from a salient function to a bus above the
 * row and into the detail's top, along the row between two details, or up from a detail into a salient function.
 */
function detailRoute(from: Rect, to: Rect, row: number): Route {
  const bus = row - BUS_ABOVE;
  if (from.y < row && to.y >= row) {
    const x1 = from.x + Math.min(from.width / 3, 48);
    const x2 = to.x + to.width / 2;
    return { d: `M ${x1} ${from.y + from.height} V ${bus} H ${x2} V ${to.y}`, mid: { x: x2, y: bus } };
  }
  if (from.y >= row && to.y >= row) {
    const [x1, x2] = to.x >= from.x ? [from.x + from.width, to.x] : [from.x, to.x + to.width];
    const y = from.y + from.height / 2;
    return { d: `M ${x1} ${y} H ${x2}`, mid: { x: (x1 + x2) / 2, y } };
  }
  const x = from.x + from.width / 2;
  return { d: `M ${x} ${from.y} V ${to.y + to.height}`, mid: { x, y: (from.y + to.y + to.height) / 2 } };
}

/**
 * The calls view (Figma 04): the salient functions an operation's code calls or reaches. A reaches edge, dashed,
 * counts the detail functions it passes; functions of other components merge into their folded component with how
 * many calls go there; an expanded function shows the detail functions it calls.
 */
export function CallsView({ view }: { view: View }) {
  const links = useLinks();
  const folded = new Set(view.folded);
  const details = new Set(view.nodes.filter(node => stateOf(node, folded) === 'detail').map(node => node.id));
  const widthOf = (node: ViewNode) => chipWidth(node.title, stateOf(node, folded) !== 'detail');
  const salient = view.nodes.filter(node => !details.has(node.id));
  const edgeLinks: LayoutLink[] = view.edges.filter(edge => !details.has(edge.from) && !details.has(edge.to)).map(edge => ({ id: edgeKey(edge), from: edge.from, to: edge.to }));
  const arranged = arrange(salient.map(node => ({ id: node.id, width: widthOf(node), height: CHIP_HEIGHT })), edgeLinks, 'LR', SPACING);
  const rects = new Map(arranged.rects);
  // The detail functions an expansion shows run in a row under the drawing, in the order the calls reach them.
  const root = view.nodes.find(node => node.bindings?.some(binding => binding.node.id === view.input.id)) ?? view.nodes[0];
  let x = (root ? (rects.get(root.id)?.x ?? 0) + widthOf(root) / 2 : 0) + DETAIL_GAP;
  const row = arranged.height + DETAIL_DROP;
  const detailStart = x;
  for (const node of view.nodes.filter(item => details.has(item.id))) {
    rects.set(node.id, { x, y: row, width: widthOf(node), height: CHIP_HEIGHT });
    x += widthOf(node) + DETAIL_GAP;
  }
  const routes = new Map(view.edges.flatMap(edge => {
    const from = rects.get(edge.from);
    const to = rects.get(edge.to);
    if (!from || !to) {
      return [];
    }
    if (!details.has(edge.from) && !details.has(edge.to)) {
      return [[edgeKey(edge), route(from, to, 0, 1, 'LR')] as const];
    }
    return [[edgeKey(edge), detailRoute(from, to, row)] as const];
  }));
  const lines: Line[] = view.edges.flatMap(edge => {
    const path = routes.get(edgeKey(edge));
    return path ? [{ id: edgeKey(edge), route: path, tone: lineOf(edge, details), select: edge.kind === 'reaches' ? edgeKey(edge) : undefined }] : [];
  });
  const collapse = view.expand.length > 0 ? links.expandHref?.(view.expand[0] ?? '') : undefined;
  const lowest = Math.max(...[...rects.values()].map(rect => rect.y + rect.height));
  const width = Math.max(arranged.width, x);
  const height = lowest;
  return (
    <Canvas about={ABOUT.callsView} label={VIEW_NAMES.calls} width={width} height={height + (collapse ? 2 * CHIP_HEIGHT : 0)}>
      <Lines width={width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {view.nodes.map(node => {
        const rect = rects.get(node.id);
        const count = view.edges.find(edge => edge.to === node.id && edge.count !== undefined)?.count;
        return rect && (
          <Placed key={node.id} rect={rect} className={styles.chipSlot}>
            <NodeChip node={node} state={stateOf(node, folded)} status={statusOf(node)} />
            {count !== undefined && (
              <span className={styles.count}>
                <Tag><Icon glyph={Merge} size="tiny" />{count}</Tag>
                <InfoHint subject={MARKS.merged}>{ABOUT.merged}</InfoHint>
              </span>
            )}
          </Placed>
        );
      })}
      {view.edges.filter(edge => edge.kind === 'reaches').map(edge => {
        const path = routes.get(edgeKey(edge));
        return path && <Marker key={edgeKey(edge)} at={path.mid}><ViaTag count={edge.attrs.via?.length ?? 0} /></Marker>;
      })}
      {collapse && (
        <Placed rect={{ x: detailStart, y: lowest + CHIP_HEIGHT, width: 120, height: CHIP_HEIGHT }} className={styles.chipSlot}>
          <ButtonLink to={collapse} variant="line" size="small"><Icon glyph={ChevronUp} size="tiny" />{SECTIONS.collapse}</ButtonLink>
        </Placed>
      )}
    </Canvas>
  );
}
