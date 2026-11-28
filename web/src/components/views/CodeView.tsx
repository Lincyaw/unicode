import { clsx } from 'clsx';
import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ABOUT, MARKS, SECTIONS, VIEW_NAMES } from '../../terms';
import { HintedHeading, Tag } from '../../ui/content';
import { ButtonLink, MarkButton } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Icon } from '../../ui/icons';
import { Canvas, Placed } from '../canvas/Canvas';
import { LineMarks, Lines, type Line } from '../canvas/Edges';
import { BADGE_SIDES } from '../canvas/markers';
import { CHIP_HEIGHT } from '../canvas/sizes';
import { ViaTag } from '../details/sections';
import { relative, scopeOf, shortPath, statusOf } from '../marks/format';
import { Code, FileCode, FlaskConical, KindIcon, Merge, Split } from '../marks/icons';
import { useLinks, useNodeActions } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { NodeRef } from '../marks/NodeRef';
import { bindingOf, layOutCode, rowLabel, type Box } from './codeLayout';
import { edgeKey } from './graphParts';
import styles from './views.module.css';

const COVERAGE_GAP = 24;
const COVERAGE_HEIGHT = 84;

/** The head of the input's box, which previews the input. */
function InputHead({ node }: { node: ViewNode }) {
  return (
    <MarkButton className={styles.fileHead} {...useNodeActions(node.id)}>
      <KindIcon node={node} size="tiny" />
      {node.form === 'file' ? shortPath(node.title) : node.title}
    </MarkButton>
  );
}

/** A box of types and functions, each with the design node it realizes. */
function FileBox({ box, input, details }: { box: Box; input: boolean; details?: number }) {
  return (
    <div className={clsx(styles.fileBox, input && 'loop-edge-still')} aria-current={input ? 'true' : undefined}>
      {box.file ? <InputHead node={box.file} /> : <span className={styles.fileHead}><Icon glyph={FileCode} size="tiny" />{shortPath(box.path ?? '')}</span>}
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
          <Icon glyph={Code} size="tiny" />+{details}
          <InfoHint subject={SECTIONS.details}>{ABOUT.details}</InfoHint>
        </Placed>
      )}
    </div>
  );
}

/** How many calls of the graph the drawn edges into or out of a folded component stand for, together. */
function mergedCount(edges: ViewEdge[], component: string): number | undefined {
  const counts = edges.filter(edge => (edge.from === component || edge.to === component) && edge.count !== undefined).map(edge => edge.count ?? 0);
  return counts.length > 0 ? counts.reduce((sum, count) => sum + count, 0) : undefined;
}

/** A folded component as a chip, centred in the height its lines need, with how many calls its edges stand for. */
function FoldedChip({ node, count }: { node: ViewNode; count?: number }) {
  return (
    <>
      <NodeChip node={node} state="folded" />
      {count !== undefined && (
        <span className={styles.count}>
          <Tag><Icon glyph={Merge} size="tiny" />{count}</Tag>
          <InfoHint subject={MARKS.merged}>{ABOUT.merged}</InfoHint>
        </span>
      )}
    </>
  );
}

/**
 * The code view (Figma 11): the input file (or type) with its types and functions and what they realize; the files
 * whose functions call into it on its left and those it calls on its right, the components called into folded, with
 * how many calls; and the cases that cover the input (codeLayout.ts).
 */
export function CodeView({ view }: { view: View }) {
  const links = useLinks();
  const layout = layOutCode(view);
  const { input } = layout;
  const lines: Line[] = layout.calls.flatMap(edge => {
    const path = layout.routes.get(edgeKey(edge));
    return path ? [{ id: edgeKey(edge), route: path, tone: edge.kind === 'reaches' ? 'reach' as const : 'plain' as const, select: edge.kind === 'reaches' ? edgeKey(edge) : undefined }] : [];
  });
  const inputRect = layout.place.get(input) ?? { x: 0, y: 0, width: input.width, height: input.height };
  const cases = view.nodes.filter(node => node.kind === 'case');
  const branches = [input.file, ...input.rows.map(row => row.node)].flatMap(node => node?.evidence?.branches ?? []);
  const height = Math.max(layout.height, inputRect.y + input.height + (cases.length > 0 ? COVERAGE_GAP + COVERAGE_HEIGHT : 0));
  return (
    <Canvas about={ABOUT.codeView} label={VIEW_NAMES.code} width={layout.width} height={height}>
      <Lines width={layout.width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {layout.boxes.map(box => {
        const rect = layout.place.get(box);
        if (!rect) {
          return null;
        }
        if (box.folded) {
          return (
            <Placed key={box.key} rect={{ ...rect, y: rect.y + (rect.height - CHIP_HEIGHT) / 2, height: CHIP_HEIGHT }} className={styles.chipSlot}>
              <FoldedChip node={box.folded} count={mergedCount(layout.calls, box.folded.id)} />
            </Placed>
          );
        }
        return <Placed key={box.key} rect={rect}><FileBox box={box} input={box === input} details={box === input ? input.file?.details : undefined} /></Placed>;
      })}
      <LineMarks
        marks={layout.calls.filter(edge => edge.kind === 'reaches').flatMap(edge => {
          const path = layout.routes.get(edgeKey(edge));
          return path ? [{ id: edgeKey(edge), route: path, sides: BADGE_SIDES, children: <ViaTag count={edge.attrs.via?.length ?? 0} /> }] : [];
        })}
        obstacles={layout.obstacles}
      />
      {cases.length > 0 && (
        <Placed rect={{ x: inputRect.x, y: inputRect.y + input.height + COVERAGE_GAP, width: input.width, height: 48 }} className={styles.coverage}>
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
