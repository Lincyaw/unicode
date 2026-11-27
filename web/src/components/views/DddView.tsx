import type { View, ViewNode } from '../../api/types';
import { ABOUT, VIEW_NAMES } from '../../terms';
import { Tag } from '../../ui/content';
import { Canvas, Placed } from '../canvas/Canvas';
import { ContainerBox, FoldedCard } from '../canvas/cards';
import { CARD_WIDTH, CHIP_HEIGHT, chipWidth, CONTAINER_HEAD, CONTAINER_PADDING, foldedHeight } from '../canvas/sizes';
import { Lines, Marker, type Line } from '../canvas/Edges';
import { arrange, routeAll, shifted, shiftedBends, TIGHT, type Link, type Rect } from '../canvas/layout';
import { textWidth } from '../canvas/measure';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { HiddenCount, UndeclaredTag } from './ArchitectureView';
import { boxEdges, componentGraph, edgeKey, toneOf } from './graphParts';
import styles from './views.module.css';

const SPACING = { node: 40, rank: 64 };
const INNER = { node: TIGHT.node + 12, rank: TIGHT.rank };
const LABEL_SIZE = 12;
const LABEL_PADDING = 16;

/** The DDD labels a type carries, as tags beside its name. */
function dddLabels(node: ViewNode): string[] {
  return node.attrs.labels ?? [];
}

function chipFor(node: ViewNode): number {
  return chipWidth(node.title) + dddLabels(node).reduce((sum, label) => sum + 8 + LABEL_PADDING + textWidth(label, LABEL_SIZE), 0);
}

/** A bounded context drawn open: its types laid out by their references inside the box. */
interface Context {
  rects: Map<string, Rect>;
  lines: Line[];
  width: number;
  height: number;
}

function context(container: ViewNode, view: View): Context {
  const types = (container.contents ?? []).filter(node => node.kind === 'type');
  const drawn = new Set(types.map(node => node.id));
  const edges = boxEdges(view.edges, drawn);
  const links: Link[] = edges.map(edge => ({ id: edge.id, from: edge.from, to: edge.to }));
  const arranged = arrange(types.map(node => ({ id: node.id, width: chipFor(node), height: CHIP_HEIGHT })), links, 'TB', INNER);
  const rects = shifted(arranged.rects, CONTAINER_PADDING, CONTAINER_HEAD);
  const routes = routeAll(links, rects, 'TB', shiftedBends(arranged.bends, CONTAINER_PADDING, CONTAINER_HEAD));
  const lines = edges.flatMap(edge => {
    const route = routes.get(edge.id);
    return route ? [{ id: edge.id, route, tone: toneOf(edge.edge) }] : [];
  });
  return { rects, lines, width: Math.max(arranged.width + 2 * CONTAINER_PADDING, CARD_WIDTH), height: arranged.height + CONTAINER_HEAD + CONTAINER_PADDING };
}

/** The DDD view (Figma 07): bounded contexts with their aggregates, values, services and repositories. */
export function DddView({ view: answer }: { view: View }) {
  const links = useLinks();
  const view = { ...answer, nodes: componentGraph(answer).nodes };
  const folded = new Set(view.folded);
  const contexts = new Map(view.nodes.filter(node => !folded.has(node.id)).map(node => [node.id, context(node, view)]));
  const boxes = view.nodes.map(node => {
    const inner = contexts.get(node.id);
    return inner ? { id: node.id, width: inner.width, height: inner.height } : { id: node.id, width: CARD_WIDTH, height: foldedHeight((node.contents ?? []).map(child => child.title)) };
  });
  const outer = componentGraph(answer).edges;
  const outerLinks: Link[] = outer.map(edge => ({ id: edgeKey(edge), from: edge.from, to: edge.to, unranked: edge.status === 'contradicted' }));
  const { rects, bends, width, height } = arrange(boxes, outerLinks, 'TB', SPACING);
  const routes = routeAll(outerLinks, rects, 'TB', bends);
  const lines: Line[] = outer.flatMap(edge => {
    const route = routes.get(edgeKey(edge));
    return route ? [{ id: edgeKey(edge), route, tone: toneOf(edge), select: edgeKey(edge) }] : [];
  });
  return (
    <Canvas about={ABOUT.ddd} label={VIEW_NAMES.ddd} width={width} height={height} footer={view.hidden > 0 ? <HiddenCount hidden={view.hidden} /> : undefined}>
      <Lines width={width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {view.nodes.map(node => {
        const rect = rects.get(node.id);
        const inner = contexts.get(node.id);
        if (!rect) {
          return null;
        }
        if (!inner) {
          return <Placed key={node.id} rect={rect}><FoldedCard node={node} names={node.contents ?? []} /></Placed>;
        }
        return (
          <Placed key={node.id} rect={rect}>
            <ContainerBox node={node} collapsible={view.expand.includes(node.id)}>
              <Lines width={inner.width} height={inner.height} lines={inner.lines} />
              {(node.contents ?? []).filter(child => inner.rects.has(child.id)).map(child => (
                <Placed key={child.id} rect={inner.rects.get(child.id) ?? { x: 0, y: 0, width: 0, height: 0 }} className={styles.chipSlot}>
                  <NodeChip node={child}>{dddLabels(child).map(label => <Tag key={label}>{label}</Tag>)}</NodeChip>
                </Placed>
              ))}
            </ContainerBox>
          </Placed>
        );
      })}
      {view.edges.filter(edge => edge.status === 'contradicted').map(edge => {
        const route = routes.get(edgeKey(edge));
        return route && <Marker key={edgeKey(edge)} at={route.mid}><UndeclaredTag edge={edge} /></Marker>;
      })}
    </Canvas>
  );
}
