import { EyeOff, TriangleAlert } from 'lucide-react';
import type { View, ViewEdge, ViewNode } from '../../api/types';
import { edgeKey } from '../../api/types';
import { ABOUT, DIAGNOSTIC_WORDS, VIEW_NAMES } from '../../terms';
import { ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { ContainerBox, ExternalCard, FoldedCard, TypeCard } from '../canvas/cards';
import { CARD_WIDTH, cardHeight, CHIP_HEIGHT, chipWidth, CONTAINER_HEAD, CONTAINER_PADDING, externalHeight, foldedHeight, typeRows, type CardRow } from '../canvas/sizes';
import { Lines, Marker, type Line } from '../canvas/Edges';
import { arrange, routeAll, type Box, type Link, type Rect } from '../canvas/layout';
import { useLinks } from '../marks/links';
import { evidenceCount, externalsBeside } from './graphParts';
import { NodeChip } from '../marks/NodeChip';
import styles from './views.module.css';

/** The type cards of an expanded container sit in this many columns, this far apart. */
const COLUMNS = 3;
const GRID_GAP = 12;
const CHIP_GAP = 8;
const SPACING = { node: 36, rank: 64 };

/** How an expanded container lays out what it holds: type cards in a grid, then its flows, properties and errors as chips. */
interface Inside {
  cards: { node: ViewNode; rows: CardRow[]; rect: Rect }[];
  chips: { node: ViewNode; rect: Rect }[];
  width: number;
  height: number;
}

function inside(container: ViewNode): Inside {
  const contents = container.contents ?? [];
  const typed = contents.filter(node => node.kind === 'type' && node.form !== 'error');
  const chipNodes = contents.filter(node => !(node.kind === 'type' && node.form !== 'error'));
  const columns = Math.max(1, Math.min(COLUMNS, typed.length));
  const gridWidth = columns * CARD_WIDTH + (columns - 1) * GRID_GAP;
  const cards: Inside['cards'] = [];
  let y = CONTAINER_HEAD;
  for (let start = 0; start < typed.length; start += columns) {
    const row = typed.slice(start, start + columns).map(node => ({ node, rows: typeRows(node) }));
    const height = Math.max(...row.map(card => cardHeight(card.rows.length)));
    row.forEach((card, column) => cards.push({ ...card, rect: { x: CONTAINER_PADDING + column * (CARD_WIDTH + GRID_GAP), y, width: CARD_WIDTH, height: cardHeight(card.rows.length) } }));
    y += height + GRID_GAP;
  }
  const chips: Inside['chips'] = [];
  let x = CONTAINER_PADDING;
  const room = Math.max(gridWidth, 0);
  if (chipNodes.length > 0) {
    for (const node of chipNodes) {
      const width = chipWidth(node.title);
      if (x > CONTAINER_PADDING && x - CONTAINER_PADDING + width > room) {
        x = CONTAINER_PADDING;
        y += CHIP_HEIGHT + CHIP_GAP;
      }
      chips.push({ node, rect: { x, y, width, height: CHIP_HEIGHT } });
      x += width + CHIP_GAP;
    }
    y += CHIP_HEIGHT + GRID_GAP;
  }
  const width = Math.max(gridWidth, ...chips.map(chip => chip.rect.x + chip.rect.width - CONTAINER_PADDING)) + 2 * CONTAINER_PADDING;
  return { cards, chips, width, height: y };
}

/** The badge on an undeclared dependency: the warning and how many imports and calls make it, with its ⓘ. */
export function UndeclaredBadge({ edge }: { edge: ViewEdge }) {
  return (
    <>
      <span className={styles.warningBadge}>
        <TriangleAlert size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
        {DIAGNOSTIC_WORDS.count(evidenceCount(edge))}
      </span>
      <InfoHint label={DIAGNOSTIC_WORDS.undeclared}>{ABOUT.undeclared}</InfoHint>
    </>
  );
}

/** The count of utility components a view does not draw, bottom left. */
export function HiddenCount({ hidden }: { hidden: number }) {
  return (
    <>
      <EyeOff size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
      <span className="t-mono">{hidden}</span>
      <InfoHint label={ABOUT.hidden}>{ABOUT.hidden}</InfoHint>
    </>
  );
}

/** The architecture view (Figma 01): components and external systems with their dependencies. */
export function ArchitectureView({ view }: { view: View }) {
  const links = useLinks();
  const folded = new Set(view.folded);
  const expandedOnRequest = new Set(view.expand);
  const insides = new Map(view.nodes.filter(node => node.kind === 'container' && !folded.has(node.id)).map(node => [node.id, inside(node)]));
  const boxes: Box[] = view.nodes.map(node => {
    const layout = insides.get(node.id);
    if (layout) {
      return { id: node.id, width: layout.width, height: layout.height };
    }
    if (node.kind === 'actor') {
      return { id: node.id, width: CARD_WIDTH, height: externalHeight(node) };
    }
    return { id: node.id, width: CARD_WIDTH, height: foldedHeight((node.contents ?? []).map(child => child.title)) };
  });
  const edgeLinks: Link[] = view.edges.map(edge => ({ id: edgeKey(edge), from: edge.from, to: edge.to, unranked: edge.status === 'contradicted' }));
  const { rects, bends, width, height } = arrange(boxes, edgeLinks, 'TB', SPACING, externalsBeside(view));
  const routes = routeAll(edgeLinks, rects, 'TB', bends);
  const lines: Line[] = view.edges.flatMap(edge => {
    const route = routes.get(edgeKey(edge));
    return route ? [{ id: edgeKey(edge), route, tone: edge.status === 'contradicted' ? 'warning' as const : 'plain' as const, select: edgeKey(edge) }] : [];
  });
  return (
    <Canvas about={ABOUT.architecture} label={VIEW_NAMES.architecture} width={width} height={height} footer={view.hidden > 0 ? <HiddenCount hidden={view.hidden} /> : undefined}>
      <Lines width={width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {view.nodes.map(node => {
        const rect = rects.get(node.id);
        if (!rect) {
          return null;
        }
        const layout = insides.get(node.id);
        if (layout) {
          return (
            <Placed key={node.id} rect={rect}>
              <ContainerBox node={node} collapsible={expandedOnRequest.has(node.id)}>
                {layout.cards.map(card => <Placed key={card.node.id} rect={card.rect}><TypeCard node={card.node} rows={card.rows} /></Placed>)}
                {layout.chips.map(chip => <Placed key={chip.node.id} rect={chip.rect} className={styles.chipSlot}><NodeChip node={chip.node} /></Placed>)}
              </ContainerBox>
            </Placed>
          );
        }
        return (
          <Placed key={node.id} rect={rect}>
            {node.kind === 'actor' ? <ExternalCard node={node} /> : <FoldedCard node={node} names={node.contents ?? []} />}
          </Placed>
        );
      })}
      {view.edges.filter(edge => edge.status === 'contradicted').map(edge => {
        const route = routes.get(edgeKey(edge));
        return route && <Marker key={edgeKey(edge)} at={route.mid}><UndeclaredBadge edge={edge} /></Marker>;
      })}
    </Canvas>
  );
}
