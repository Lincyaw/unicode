import { clsx } from 'clsx';
import type { View, ViewNode } from '../../api/types';
import { ABOUT, MARKS, SECTIONS, VIEW_NAMES } from '../../terms';
import { HintedHeading, Tag } from '../../ui/content';
import { ButtonLink, MarkButton } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Canvas, Placed } from '../canvas/Canvas';
import { CHIP_HEIGHT, chipWidth } from '../canvas/sizes';
import { Lines, Marker, type Line } from '../canvas/Edges';
import { route, type Rect } from '../canvas/layout';
import { monoWidth } from '../canvas/measure';
import { ViaTag } from '../details/sections';
import { relative, scopeOf, shortPath, statusOf } from '../marks/format';
import { Code, FlaskConical, KindIcon, Merge, Split } from '../marks/icons';
import { useLinks, useNodeActions } from '../marks/links';
import { NodeRef } from '../marks/NodeRef';
import { NodeChip } from '../marks/NodeChip';
import { edgeKey } from './graphParts';
import styles from './views.module.css';
import { ICON_SMALL, Icon } from '../../ui/icons';

const CALLER_GAP = 44;
const CALLEE_GAP = 72;
const CALLEE_ROW_GAP = 20;
const COVERAGE_HEIGHT = 84;
const COUNT_ROOM = 72;
const PADDING = 12;
const HEAD = 18;
const ROW = 30;
const GAP = 6;
const INDENT = 16;
const BINDING_GAP = 24;
const MIN_WIDTH = 240;

/** A row of a file's box: a type or a function, indented under its type, at its place in the box. */
interface Row {
  node: ViewNode;
  depth: number;
  rect: Rect;
}

/** The name a row shows: a method without its type's name, which the row above already gives. */
function rowLabel(node: ViewNode, depth: number): string {
  return depth > 0 ? node.title.split('.').at(-1) ?? node.title : node.title;
}

function bindingOf(node: ViewNode): ViewNode | undefined {
  return node.bindings?.[0]?.node;
}

/** A file drawn as a box: its head, then its types and functions, each with what it realizes. */
interface FileBox {
  file: ViewNode;
  rows: Row[];
  details?: Rect;
  width: number;
  height: number;
}

/** A file's box, laid out with a row for its undrawn detail functions when `details` counts any (only the input's box does). */
function fileBox(file: ViewNode, details: number): FileBox {
  const items = (file.contents ?? []).flatMap(node => [{ node, depth: 0 }, ...(node.contents ?? []).filter(child => child.kind === 'op').map(child => ({ node: child, depth: 1 }))]);
  const widths = items.map(({ node, depth }) => {
    const bound = bindingOf(node);
    return depth * INDENT + chipWidth(rowLabel(node, depth)) + (bound ? BINDING_GAP + ICON_SMALL + 4 + monoWidth(bound.title, 12) + 16 : 0);
  });
  const width = Math.max(MIN_WIDTH, monoWidth(shortPath(file.title), 12) + 60, ...widths.map(item => item + 2 * PADDING));
  let y = PADDING + HEAD + GAP + 4;
  const rows = items.map(({ node, depth }) => {
    const rect = { x: PADDING + depth * INDENT, y, width: width - 2 * PADDING - depth * INDENT, height: ROW };
    y += ROW + GAP;
    return { node, depth, rect };
  });
  const detailsRect = details > 0 ? { x: PADDING + INDENT, y, width: 80, height: CHIP_HEIGHT - 8 } : undefined;
  if (detailsRect) {
    y += detailsRect.height + GAP;
  }
  return { file, rows, details: detailsRect, width, height: y + PADDING - GAP };
}

/** The head of a file's box, which previews the file. */
function FileHead({ file }: { file: ViewNode }) {
  return (
    <MarkButton className={styles.fileHead} {...useNodeActions(file.id)}>
      <KindIcon node={file} size="tiny" />
      {shortPath(file.title)}
    </MarkButton>
  );
}

/**
 * The code view (Figma 11): a file with its types and functions and what they realize, the files whose functions call
 * them or are called, the components called into (folded, with how many calls), and the cases that cover the file.
 */
export function CodeView({ view }: { view: View }) {
  const links = useLinks();
  const folded = new Set(view.folded);
  const files = view.nodes.filter(node => node.kind === 'container' && node.form === 'file');
  const cases = view.nodes.filter(node => node.kind === 'case');
  const containers = view.nodes.filter(node => folded.has(node.id));
  const boxesByFile = new Map(files.map(file => [file.id, fileBox(file, file.id === view.input.id ? file.details ?? 0 : 0)]));
  const owner = new Map<string, string>();
  for (const box of boxesByFile.values()) {
    box.rows.forEach(row => owner.set(row.node.id, box.file.id));
  }
  containers.forEach(container => (container.contents ?? []).forEach(child => owner.set(child.id, container.id)));
  containers.forEach(container => owner.set(container.id, container.id));
  const calls = view.edges.filter(edge => edge.kind === 'calls' || edge.kind === 'reaches');
  // Files whose functions call into the input stand above it; what it calls stands in a column to its right.
  const callers = new Set(calls.filter(edge => owner.get(edge.to) === view.input.id).flatMap(edge => owner.get(edge.from) ?? []));
  callers.delete(view.input.id);
  const rects = new Map<string, Rect>();
  let y = 0;
  for (const id of callers) {
    const box = boxesByFile.get(id);
    if (box) {
      rects.set(id, { x: 0, y, width: box.width, height: box.height });
      y += box.height + CALLER_GAP;
    }
  }
  const input = boxesByFile.get(view.input.id);
  const inputTop = y;
  if (input) {
    rects.set(view.input.id, { x: 0, y, width: input.width, height: input.height });
  }
  const column = (input?.width ?? 0) + CALLEE_GAP;
  let calleeY = Math.max(0, inputTop - CALLER_GAP);
  const callees = [
    ...[...boxesByFile.values()].filter(box => box.file.id !== view.input.id && !callers.has(box.file.id)).map(box => ({ id: box.file.id, width: box.width, height: box.height })),
    ...containers.map(node => ({ id: node.id, width: chipWidth(node.title), height: CHIP_HEIGHT })),
  ];
  for (const box of callees) {
    rects.set(box.id, { x: column, y: calleeY, width: box.width, height: box.height });
    calleeY += box.height + CALLEE_ROW_GAP;
  }
  const coverageHeight = cases.length > 0 ? COVERAGE_HEIGHT : 0;
  // A folded component's count stands after its chip.
  const width = Math.max(0, ...[...rects].map(([id, rect]) => rect.x + rect.width + (folded.has(id) ? COUNT_ROOM : 0)));
  const height = Math.max(calleeY, inputTop + (input?.height ?? 0) + coverageHeight);
  const placeOf = (id: string): Rect | undefined => {
    const box = owner.get(id);
    const at = box === undefined ? undefined : rects.get(box);
    if (!at) {
      return undefined;
    }
    if (folded.has(box ?? '')) {
      return { ...at, width: chipWidth(view.nodes.find(node => node.id === box)?.title ?? '') };
    }
    const row = boxesByFile.get(box ?? '')?.rows.find(item => item.node.id === id);
    return row ? { x: at.x + row.rect.x, y: at.y + row.rect.y, width: chipWidth(rowLabel(row.node, row.depth)), height: row.rect.height } : at;
  };
  const routed = calls.flatMap(edge => {
    const from = placeOf(edge.from);
    const to = placeOf(edge.to);
    return from && to ? [{ edge, route: route(from, to, 0, 1, to.x >= from.x + from.width ? 'LR' : 'TB') }] : [];
  });
  const lines: Line[] = routed.map(({ edge, route: path }) => ({ id: edgeKey(edge), route: path, tone: edge.kind === 'reaches' ? 'reach' : 'plain', select: edge.kind === 'reaches' ? edgeKey(edge) : undefined }));
  const inputRect = rects.get(view.input.id);
  const branches = view.nodes.flatMap(node => [node, ...(node.contents ?? []).flatMap(child => [child, ...(child.contents ?? [])])]).flatMap(node => node.evidence?.branches ?? []);
  return (
    <Canvas about={ABOUT.codeView} label={VIEW_NAMES.code} width={width} height={height}>
      <Lines width={width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {[...boxesByFile.values()].map(box => {
        const rect = rects.get(box.file.id);
        const isInput = box.file.id === view.input.id;
        return rect && (
          <Placed key={box.file.id} rect={{ ...rect, height: box.height }}>
            <div className={clsx(styles.fileBox, isInput && 'loop-edge-still')} aria-current={isInput ? 'true' : undefined}>
              <FileHead file={box.file} />
              {box.rows.map(row => {
                const bound = bindingOf(row.node);
                return (
                  <Placed key={row.node.id} rect={row.rect} className={styles.fileRow}>
                    <NodeChip node={row.node} label={rowLabel(row.node, row.depth)} status={statusOf(row.node)} className={styles.rowChip} />
                    {bound && (
                      <NodeRef id={bound.id} className={styles.binding}>
                        <KindIcon node={bound} size="tiny" />
                        {relative(bound.id, scopeOf(bound.id))}
                      </NodeRef>
                    )}
                  </Placed>
                );
              })}
              {box.details && (
                <Placed rect={box.details} className={styles.detailsCount}>
                  <Icon glyph={Code} size="tiny" />+{box.file.details}
                  <InfoHint subject={SECTIONS.details}>{ABOUT.details}</InfoHint>
                </Placed>
              )}
            </div>
          </Placed>
        );
      })}
      {containers.map(node => {
        const rect = rects.get(node.id);
        const count = view.edges.find(edge => edge.to === node.id)?.count;
        return rect && (
          <Placed key={node.id} rect={{ ...rect, width: chipWidth(node.title) }} className={styles.chipSlot}>
            <NodeChip node={node} state="folded" />
            {count !== undefined && (
              <span className={styles.count}>
                <Tag><Icon glyph={Merge} size="tiny" />{count}</Tag>
                <InfoHint subject={MARKS.merged}>{ABOUT.merged}</InfoHint>
              </span>
            )}
          </Placed>
        );
      })}
      {routed.filter(({ edge }) => edge.kind === 'reaches').map(({ edge, route: path }) => (
        <Marker key={edgeKey(edge)} at={path.mid}><ViaTag count={edge.attrs.via?.length ?? 0} /></Marker>
      ))}
      {input && inputRect && cases.length > 0 && (
        <Placed rect={{ x: inputRect.x, y: inputRect.y + input.height + 24, width: input.width, height: 48 }} className={styles.coverage}>
          <HintedHeading rank="h3" className="t-micro c-muted" title={SECTIONS.coveredBy} about={ABOUT.coveredBy} />
          <span className={styles.coverageChips}>
            {branches.map(branch => <Tag key={branch.label}><Icon glyph={Split} size="tiny" />{branch.label} {branch.cases}</Tag>)}
            {cases.map(item => (
              <ButtonLink key={item.id} to={links.entityHref(item.id)} variant="line" size="small"><Icon glyph={FlaskConical} size="tiny" />{item.title}</ButtonLink>
            ))}
          </span>
        </Placed>
      )}
    </Canvas>
  );
}
