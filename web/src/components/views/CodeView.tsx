import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import type { View, ViewEdge, ViewNode } from '../../api/types';
import { ABOUT, SECTIONS, VIEW_NAMES } from '../../terms';
import { HintedHeading, Tag } from '../../ui/content';
import { ButtonLink, MarkButton } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Icon } from '../../ui/icons';
import { Canvas, Placed } from '../canvas/Canvas';
import { LineMarks, Lines, type Line, type LineMark } from '../canvas/Edges';
import { shortPath, statusOf } from '../marks/format';
import { Code, FileCode, FlaskConical, KindIcon, Split } from '../marks/icons';
import { useLinks, useNodeActions } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { NodeRef } from '../marks/NodeRef';
import { CallMark } from './CallMark';
import { bindingLabel, bindingOf, layOutCode, rowLabel, type Box } from './codeLayout';
import { edgeKey, markOf } from './graphParts';
import styles from './views.module.css';

const COVERAGE_GAP = 24;
const COVERAGE_HEIGHT = 84;

/** The head of a box: the input, or the file of the other functions; a click previews it, a double click enters it. */
function BoxHead({ id, children }: { id: string; children: ReactNode }) {
  return <MarkButton className={styles.fileHead} {...useNodeActions(id)}>{children}</MarkButton>;
}

/** A box of types and functions, each with the design node it realizes. */
function FileBox({ box, details }: { box: Box; details?: number }) {
  return (
    <div className={clsx(styles.fileBox, box.input && 'loop-edge-still')} aria-current={box.input ? 'true' : undefined}>
      {box.input && (
        <BoxHead id={box.input.id}>
          <KindIcon node={box.input} size="tiny" />
          {box.input.form === 'file' ? shortPath(box.input.title) : box.input.title}
        </BoxHead>
      )}
      {box.file && <BoxHead id={box.file.id}><Icon glyph={FileCode} size="tiny" />{shortPath(box.file.path)}</BoxHead>}
      {box.rows.map(row => {
        const bound = bindingOf(row.node);
        return (
          <Placed key={row.node.id} rect={row.rect} className={styles.fileRow}>
            <NodeChip node={row.node} label={rowLabel(row.node, row.depth)} status={statusOf(row.node)} className={styles.rowChip} />
            {bound && (
              <NodeRef id={bound.id} className={styles.binding}>
                <KindIcon node={bound} size="tiny" />
                {bindingLabel(bound)}
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

/**
 * The code view (Figma 11): the input file (or type) with its types and functions and what they realize; the files
 * whose functions call into it on its left and those it calls on its right, each headed by a link to the file; the
 * components called into, folded; and the cases that cover the input (codeLayout.ts). A reaches line carries how many
 * detail functions it passes, a merged line how many calls it stands for.
 */
export function CodeView({ view }: { view: View }) {
  const links = useLinks();
  const layout = layOutCode(view);
  const { input, inputNode } = layout;
  const routeOf = (edge: ViewEdge) => {
    const path = layout.routes.get(edgeKey(edge));
    if (!path) {
      throw new Error(`the code view laid out no line for ${edgeKey(edge)}`);
    }
    return path;
  };
  const lines: Line[] = layout.calls.map(edge => ({
    id: edgeKey(edge), route: routeOf(edge), tone: edge.kind === 'reaches' ? 'reach' : 'plain', select: edge.kind === 'reaches' ? edgeKey(edge) : undefined,
  }));
  const marks: LineMark[] = layout.calls.filter(edge => markOf(edge) !== undefined)
    .map(edge => ({ id: edgeKey(edge), route: routeOf(edge), sides: ['center'], children: <CallMark edge={edge} /> }));
  const inputRect = layout.boxes[0]?.rect;
  if (!inputRect) {
    throw new Error('the code view laid out no box for its input');
  }
  const cases = view.nodes.filter(node => node.kind === 'case');
  const branches = [inputNode, ...input.rows.map(row => row.node)].flatMap((node: ViewNode) => node.evidence?.branches ?? []);
  const height = Math.max(layout.height, inputRect.y + input.height + (cases.length > 0 ? COVERAGE_GAP + COVERAGE_HEIGHT : 0));
  return (
    <Canvas about={ABOUT.codeView} label={VIEW_NAMES.code} width={layout.width} height={height}>
      <Lines width={layout.width} height={height} lines={lines} selected={links.selected} onSelect={links.select} />
      {layout.boxes.map(({ box, rect }) => (box.folded
        ? (
          // A folded component is as tall as the lines that meet it need, so each of them starts on it.
          <Placed key={box.key} rect={rect} className={styles.chipSlot}>
            <NodeChip node={box.folded} state="folded" />
          </Placed>
        )
        : <Placed key={box.key} rect={rect}><FileBox box={box} details={box.input?.details} /></Placed>))}
      <LineMarks marks={marks} obstacles={layout.boxes.map(({ rect }) => rect)} />
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
