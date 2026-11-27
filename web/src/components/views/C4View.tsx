import { clsx } from 'clsx';
import type { CSSProperties, ReactNode } from 'react';
import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ABOUT, VIEW_NAMES } from '../../terms';
import { InfoHint } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { Lines, Marker, type Line } from '../canvas/Edges';
import { arrange, routeAll, shifted, type Link, type Rect } from '../canvas/layout';
import { linesOf, monoWidth, textWidth } from '../canvas/measure';
import { KindIcon } from '../marks/icons';
import { useLinks, useNodeActions, useSelectedBox } from '../marks/links';
import { Above } from '../marks/Above';
import { CoverButton } from '../marks/CoverButton';
import { StatusDot } from '../marks/StatusDot';
import { HiddenCount, UndeclaredBadge } from './ArchitectureView';
import { edgeKey, toneOf } from './graphParts';
import styles from './views.module.css';

const CARD = 216;
const PADDING = 12;
const HEAD = 18;
const SUMMARY_LINE = 21;
const PATH_LINE = 17;
const BOUNDARY_HEAD = 44;
const BOUNDARY_PADDING = 20;
const INNER = { node: 24, rank: 92 };
const OUTER = { node: 48, rank: 112 };

/** How tall a C4 card is: its head, its summary wrapped in the card, and its paths. */
function cardHeight(node: ViewNode): number {
  const summaryLines = node.summary === '' ? 0 : linesOf(textWidth(node.summary, 13), CARD - 2 * PADDING);
  const paths = (node.attrs.paths ?? []).length;
  return 2 + 2 * PADDING + HEAD + (summaryLines > 0 ? 8 + summaryLines * SUMMARY_LINE : 0) + (paths > 0 ? 4 + paths * PATH_LINE : 0);
}

function C4Card({ node, style }: { node: ViewNode; style?: CSSProperties }) {
  const box = useSelectedBox(node.id);
  const external = node.kind === 'actor' && node.form === 'external';
  return (
    <div className={clsx(styles.c4Card, box.selected && 'loop-edge-still')} data-external={external} data-person={node.form === 'person'} style={style} {...box.props}>
      <span className={styles.c4Head}>
        {node.form !== 'person' && <StatusDot status={node.status} />}
        <CoverButton id={node.id}>
          <KindIcon node={node} size="small" />
          <span className={styles.c4Title}>{node.title}</span>
        </CoverButton>
        <Above><InfoHint subject={node.id}>{node.id}</InfoHint></Above>
      </span>
      {node.summary !== '' && <span className={styles.c4Summary}>{node.summary}</span>}
      {(node.attrs.paths ?? []).map(path => <span key={path} className={styles.c4Path}>{path}</span>)}
    </div>
  );
}

/** The system's dashed boundary around its containers, with its head. */
function Boundary({ boundary }: { boundary: ViewNode }) {
  const box = useSelectedBox(boundary.id);
  return <div className={clsx(styles.boundary, box.selected && 'loop-edge-still')} {...box.props}><BoundaryHead boundary={boundary} /></div>;
}

/** The head of the system's boundary, which previews the system. */
function BoundaryHead({ boundary }: { boundary: ViewNode }) {
  return (
    <button type="button" className={styles.boundaryHead} {...useNodeActions(boundary.id)}>
      <StatusDot status={boundary.status} />
      <KindIcon node={boundary} size="small" />
      {boundary.title}
    </button>
  );
}

/**
 * Re-packs each rank of the containers so that those calling out of the system come last, nearest the external
 * systems beside the boundary, and their lines leave without crossing another container.
 */
function usersLast(rects: Map<string, Rect>, users: ReadonlySet<string>) {
  const ranks = Map.groupBy([...rects], ([, rect]) => Math.round(rect.y));
  for (const rank of ranks.values()) {
    const ordered = [...rank].sort(([a, ra], [b, rb]) => Number(users.has(a)) - Number(users.has(b)) || ra.x - rb.x);
    const gaps = rank.map(([, rect]) => rect).sort((a, b) => a.x - b.x);
    let x = gaps[0]?.x ?? 0;
    const gap = gaps.length > 1 ? (gaps[1]?.x ?? 0) - ((gaps[0]?.x ?? 0) + (gaps[0]?.width ?? 0)) : 0;
    for (const [id, rect] of ordered) {
      rects.set(id, { ...rect, x });
      x += rect.width + gap;
    }
  }
}

/** The operations an edge carries, as its label reads: `reserve · release`. */
function labelOf(edge: ViewEdge): string {
  return (edge.labels ?? []).join(' · ');
}

/**
 * The C4 view (Figma 12): at the container level, the persons, the system's boundary around its containers, and the
 * external systems, each edge labelled with the operations called through it; at the context level, the persons, the
 * system and the external systems.
 */
export function C4View({ view, toolbar }: { view: View; toolbar: ReactNode }) {
  const links = useLinks();
  const system = view.nodes.find(node => node.kind === 'container' && node.form === 'system');
  const inside = new Set(view.edges.filter(edge => edge.kind === 'contains' && edge.from === system?.id).map(edge => edge.to));
  const boundary = system !== undefined && inside.size > 0 ? system : undefined;
  const containers = view.nodes.filter(node => inside.has(node.id));
  const outerNodes = view.nodes.filter(node => !inside.has(node.id) && node !== boundary);
  const drawnEdges = view.edges.filter(edge => edge.kind !== 'contains');

  const innerLinks: Link[] = drawnEdges.filter(edge => inside.has(edge.from) && inside.has(edge.to)).map(edge => ({ id: edgeKey(edge), from: edge.from, to: edge.to, unranked: edge.status === 'contradicted' }));
  const inner = arrange(containers.map(node => ({ id: node.id, width: CARD, height: cardHeight(node) })), innerLinks, 'TB', INNER);
  usersLast(inner.rects, new Set(drawnEdges.filter(edge => inside.has(edge.from) && !inside.has(edge.to) && edge.to !== boundary?.id).map(edge => edge.from)));
  const boundaryBox = boundary && { id: boundary.id, width: inner.width + 2 * BOUNDARY_PADDING, height: inner.height + BOUNDARY_HEAD + BOUNDARY_PADDING };
  const lift = (id: string) => (inside.has(id) && boundary ? boundary.id : id);
  const outerLinks: Link[] = drawnEdges.flatMap(edge => {
    const from = lift(edge.from);
    const to = lift(edge.to);
    return from === to ? [] : [{ id: edgeKey(edge), from, to }];
  });
  const besides = new Map(outerNodes.filter(node => node.form === 'external' && boundary).map(node => [node.id, boundary?.id ?? '']));
  const outer = arrange([
    ...outerNodes.map(node => ({ id: node.id, width: Math.max(CARD, monoWidth(node.title, 12) + 80), height: cardHeight(node) })),
    ...(boundaryBox ? [boundaryBox] : []),
  ], outerLinks, 'TB', OUTER, besides);
  const rects = new Map(outer.rects);
  const boundaryRect = boundary && outer.rects.get(boundary.id);
  if (boundaryRect) {
    for (const [id, rect] of shifted(inner.rects, boundaryRect.x + BOUNDARY_PADDING, boundaryRect.y + BOUNDARY_HEAD)) {
      rects.set(id, rect);
    }
    // An external system stands level with the container that uses it.
    for (const id of besides.keys()) {
      const user = drawnEdges.find(edge => edge.to === id && inside.has(edge.from));
      const anchor = user && rects.get(user.from);
      const rect = rects.get(id);
      if (anchor && rect) {
        rects.set(id, { ...rect, y: anchor.y });
      }
    }
  }
  const routes = routeAll(drawnEdges.map(edge => ({ id: edgeKey(edge), from: edge.from, to: edge.to })), rects);
  const lines: Line[] = drawnEdges.flatMap(edge => {
    const path = routes.get(edgeKey(edge));
    return path ? [{ id: edgeKey(edge), route: path, tone: toneOf(edge), select: edge.kind === 'depends' ? edgeKey(edge) : undefined }] : [];
  });
  const width = Math.max(...[...rects.values()].map(rect => rect.x + rect.width));
  const height = Math.max(...[...rects.values()].map(rect => rect.y + rect.height));
  return (
    <Canvas about={ABOUT.c4} label={VIEW_NAMES.c4} width={width} height={height} toolbar={toolbar} footer={view.hidden > 0 ? <HiddenCount hidden={view.hidden} /> : undefined}>
      {boundary && boundaryRect && (
        <Placed rect={boundaryRect}>
          <Boundary boundary={boundary} />
        </Placed>
      )}
      <Lines width={width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {view.nodes.filter(node => node !== boundary).map(node => {
        const rect = rects.get(node.id);
        return rect && <Placed key={node.id} rect={rect}><C4Card node={node} /></Placed>;
      })}
      {drawnEdges.map(edge => {
        const path = routes.get(edgeKey(edge));
        if (!path) {
          return null;
        }
        const from = rects.get(edge.from);
        const to = rects.get(edge.to);
        const vertical = from !== undefined && to !== undefined && from.x < to.x + to.width && to.x < from.x + from.width;
        // A label keeps to the side its line leans toward, so the labels of lines leaving one box spread apart.
        const leans = from !== undefined && to !== undefined && to.x + to.width / 2 < from.x + from.width / 2 - 1 ? 'left' : 'right';
        return (
          <Marker key={edgeKey(edge)} at={path.mid} side={edge.status === 'contradicted' ? 'center' : vertical ? leans : 'above'}>
            {edge.status === 'contradicted' ? <UndeclaredBadge edge={edge} /> : <span className={styles.edgeLabel}>{labelOf(edge)}</span>}
          </Marker>
        );
      })}
    </Canvas>
  );
}

