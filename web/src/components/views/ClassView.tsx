import type { ReactNode } from 'react';
import type { View } from '../../api/types';
import { ABOUT, VIEW_NAMES } from '../../terms';
import { Canvas, Placed } from '../canvas/Canvas';
import { FoldedCard, TypeCard } from '../canvas/cards';
import { useUnfolded } from '../canvas/unfolded';
import { CARD_WIDTH, cardHeight, foldedHeight, typeRows, unfoldedTypeRows } from '../canvas/sizes';
import { Lines, type Line } from '../canvas/Edges';
import { layOut, type LayeredLink } from '../canvas/layered';
import { useLinks } from '../marks/links';
import { boxEdges, fromField } from './graphParts';

const SPACING = { node: 32, rank: 44 };

/**
 * The class view (Figma 08): the types of a component as cards with their fields and operations, and the types of
 * other components they refer to inside those components' folded cards. A reference from a field is a solid line, one
 * from an operation's signature dashed.
 */
export function ClassView({ view, toolbar }: { view: View; toolbar: ReactNode }) {
  const links = useLinks();
  const { unfolded, unfold } = useUnfolded();
  const folded = new Set(view.folded);
  const cards = view.nodes.map(node => ({ node, rows: folded.has(node.id) ? [] : unfolded.has(node.id) ? unfoldedTypeRows(node) : typeRows(node) }));
  const drawn = new Set(view.nodes.map(node => node.id));
  const byId = new Map(view.nodes.flatMap(node => [[node.id, node] as const, ...(node.contents ?? []).map(child => [child.id, child] as const)]));
  const edges = boxEdges(view.edges, drawn);
  const edgeLinks: LayeredLink[] = edges.map(edge => ({ id: edge.id, from: edge.from, to: edge.to }));
  const boxes = cards.map(({ node, rows }) => ({
    id: node.id,
    width: CARD_WIDTH,
    height: folded.has(node.id) ? foldedHeight(node.contents ?? []) : cardHeight(rows.length),
  }));
  // A folded component or an error that one card alone refers to stands beside that card.
  const sources = Map.groupBy(edges, edge => edge.to);
  const beside = new Map(view.nodes.flatMap(node => {
    const into = sources.get(node.id) ?? [];
    const leaf = folded.has(node.id) || (node.kind === 'type' && node.form === 'error');
    return leaf && into.length === 1 && into[0] && !edges.some(edge => edge.from === node.id) ? [[node.id, into[0].from] as const] : [];
  }));
  const { rects, routes, width, height } = layOut(boxes, edgeLinks, 'TB', SPACING, { beside });
  const lines: Line[] = edges.flatMap(edge => {
    const route = routes.get(edge.id);
    const solid = edge.edge.kind !== 'refers' || fromField(edge.edge, byId);
    return route ? [{ id: edge.id, route, tone: solid ? 'plain' as const : 'dashed' as const }] : [];
  });
  return (
    <Canvas about={ABOUT.class} label={VIEW_NAMES.class} width={width} height={height} toolbar={toolbar}>
      <Lines width={width} height={height} lines={lines} selected={links.selected} />
      {cards.map(({ node, rows }) => {
        const rect = rects.get(node.id);
        return rect && (
          <Placed key={node.id} rect={rect}>
            {folded.has(node.id) ? <FoldedCard node={node} names={node.contents ?? []} /> : <TypeCard node={node} rows={rows} onUnfold={() => unfold(node.id)} />}
          </Placed>
        );
      })}
    </Canvas>
  );
}
