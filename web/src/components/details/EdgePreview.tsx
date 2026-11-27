import type { EdgeEntity } from '../../api/types';
import { ABOUT, EDGE_KINDS, SECTIONS, VIEW_NAMES } from '../../terms';
import { Tag } from '../../ui/content';
import { ButtonLink } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Icon } from '../../ui/icons';
import { messageOf, placeOf, stepName } from '../marks/format';
import { ArrowRight, ArrowUpRight, ChevronRight, CornerDownRight } from '../marks/icons';
import { useLinks } from '../marks/links';
import { StatusDot } from '../marks/StatusDot';
import { ViewName } from '../marks/ViewName';
import { statusWord } from './nodeWords';
import { NodeLink } from '../marks/NodeLink';
import { DiagnosticTag, NodeRow, Section } from './sections';
import styles from './details.module.css';

/**
 * The preview of an edge (Figma 04): what joins what, the nodes its attributes name (the detail functions a reaches
 * edge passes), its end, the steps that trace to it, and the views that expand it.
 */
export function EdgePreview({ edge }: { edge: EdgeEntity }) {
  const links = useLinks();
  const { fromNode, toNode } = edge;
  const end = statusWord(toNode);
  const expand = edge.views.find(view => view.kind === 'calls' && view.expand.length > 0);
  const others = edge.views.filter(view => view !== expand);
  return (
    <div className={styles.preview}>
      <div className={styles.previewTop}>
        <div className={styles.previewHead}>
          <StatusDot status={edge.edge.status} />
          <Icon glyph={CornerDownRight} />
          <span className={styles.identity}>{EDGE_KINDS[edge.edge.kind]}</span>
          <InfoHint subject={EDGE_KINDS[edge.edge.kind]}>{`${edge.edge.origin.path}:${edge.edge.origin.line}`}</InfoHint>
        </div>
        <p className={styles.ends}>
          <NodeLink node={fromNode} tone="inherit" />
          <Icon glyph={ArrowRight} size="tiny" />
          <NodeLink node={toNode} tone="inherit" />
        </p>
      </div>
      {edge.nodes.length > 0 && (
        <Section title={edge.edge.kind === 'reaches' ? SECTIONS.via : SECTIONS.imports} about={edge.edge.kind === 'reaches' ? ABOUT.reachesCount : undefined}>
          {edge.nodes.map(node => <NodeRow key={node.id} node={node} icon dot={false} trailing={placeOf(node.origin)} />)}
        </Section>
      )}
      <Section title={SECTIONS.end}>
        <NodeRow node={toNode} trailing={end !== undefined ? <Tag>{end}</Tag> : undefined} />
      </Section>
      {edge.diagnostics.length > 0 && <Section title={SECTIONS.diagnostics}>{edge.diagnostics.map(diagnostic => <DiagnosticTag key={diagnostic.code} diagnostic={diagnostic} />)}</Section>}
      {edge.steps.length > 0 && (
        <Section title={SECTIONS.steps}>
          {edge.steps.map(step => (
            <NodeRow
              key={step.id} node={step} icon dot={false} label={stepName(step)}
              trailing={<span className={styles.ends}>{step.step?.sender?.title}<Icon glyph={ArrowRight} size="tiny" />{messageOf(step)}</span>}
            />
          ))}
        </Section>
      )}
      {(expand !== undefined || others.length > 0) && (
        <Section title={SECTIONS.views}>
          <div className={styles.chips}>
            {expand && <ButtonLink to={links.viewHref(expand)} variant="line" size="small"><Icon glyph={ChevronRight} size="tiny" />{SECTIONS.expand}</ButtonLink>}
            {others.map(view => (
              <ButtonLink key={`${view.kind} ${view.input.id} ${view.select ?? ''}`} to={links.viewHref(view)} variant="line" size="small">
                <Icon glyph={ArrowUpRight} size="tiny" />
                <ViewName view={VIEW_NAMES[view.kind]} input={view.select?.includes('#') ? `#${view.select.split('#').at(-1) ?? ''}` : view.input.title} />
              </ButtonLink>
            ))}
          </div>
        </Section>
      )}
    </div>
  );
}
