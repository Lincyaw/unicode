import { ArrowRight, ArrowUpRight, CornerDownRight, Plus } from 'lucide-react';
import type { EdgeEntity } from '../../api/types';
import { ABOUT, SECTIONS, VIEW_NAMES } from '../../terms';
import { Badge } from '../marks/Badge';
import { messageOf, placeOf, stepName } from '../marks/format';
import { useLinks } from '../marks/links';
import { StatusDot } from '../marks/StatusDot';
import { statusWord } from './nodeWords';
import { NodeLink } from '../marks/NodeLink';
import { DiagnosticTag, NodeRow, Section } from './sections';
import styles from './details.module.css';
import { ICON_SIZE, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';

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
          <CornerDownRight size={ICON_SIZE} strokeWidth={ICON_STROKE} aria-hidden="true" />
          <span className={styles.identity}>{edge.edge.kind}</span>
          <InfoHint label={edge.edge.kind}>{`${edge.edge.origin.path}:${edge.edge.origin.line}`}</InfoHint>
        </div>
        <p className={styles.ends}>
          <NodeLink node={fromNode} tone="inherit" />
          <ArrowRight size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
          <NodeLink node={toNode} tone="inherit" />
        </p>
      </div>
      {edge.nodes.length > 0 && (
        <Section title={edge.edge.kind === 'reaches' ? SECTIONS.via : SECTIONS.imports} about={edge.edge.kind === 'reaches' ? ABOUT.reachesCount : undefined}>
          {edge.nodes.map(node => <NodeRow key={node.id} node={node} icon dot={false} trailing={placeOf(node.origin)} />)}
        </Section>
      )}
      <Section title={SECTIONS.end}>
        <NodeRow node={toNode} trailing={end !== undefined ? <Badge tone="neutral">{end}</Badge> : undefined} />
      </Section>
      {edge.diagnostics.length > 0 && <Section title={SECTIONS.diagnostics}>{edge.diagnostics.map(diagnostic => <DiagnosticTag key={diagnostic.code} diagnostic={diagnostic} />)}</Section>}
      {edge.steps.length > 0 && (
        <Section title={SECTIONS.steps}>
          {edge.steps.map(step => (
            <NodeRow key={step.id} node={step} icon dot={false} label={stepName(step)} trailing={`${step.step?.sender?.title ?? ''} -> ${messageOf(step)}`} />
          ))}
        </Section>
      )}
      {(expand !== undefined || others.length > 0) && (
        <Section title={SECTIONS.actions}>
          <div className={styles.chips}>
            {expand && <Badge tone="info" icon={Plus} to={links.viewHref(expand)}>{SECTIONS.expand}</Badge>}
            {others.map(view => <Badge key={`${view.kind} ${view.input.id} ${view.select ?? ''}`} tone="info" icon={ArrowUpRight} to={links.viewHref(view)}>{VIEW_NAMES[view.kind]} {view.select?.includes('#') ? `#${view.select.split('#').at(-1) ?? ''}` : view.input.title}</Badge>)}
          </div>
        </Section>
      )}
    </div>
  );
}
