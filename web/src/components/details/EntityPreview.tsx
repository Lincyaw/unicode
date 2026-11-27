import type { Entity } from '../../api/types';
import { stepLine } from '../marks/format';
import { CaseSections, CheckedSections, CodeSections, ContainerSections, OpSections, StepSections, TypeSections } from './kindSections';
import { PreviewHead, ViewLinks } from './sections';
import styles from './details.module.css';

/** The sections of a node by its kind, as the preview lists them under its head. */
function KindSections({ entity }: { entity: Entity }) {
  const { node } = entity;
  if (node.layer === 'code') {
    return <CodeSections entity={entity} />;
  }
  switch (node.kind) {
    case 'container':
    case 'actor':
      return <ContainerSections entity={entity} />;
    case 'type':
      return <TypeSections entity={entity} preview />;
    case 'op':
      return <OpSections entity={entity} />;
    case 'step':
      return <StepSections entity={entity} />;
    case 'case':
      return <CaseSections entity={entity} />;
    case 'flow':
    case 'property':
      return <CheckedSections entity={entity} />;
    case 'observation':
    case 'run':
    case 'result':
    case 'review':
      return null;
  }
}

/**
 * The right pane's preview of a node (docs/design.md §7.5): its head with the ↗ into its page, the line that says what
 * it is, and the sections of its kind; the related views close it.
 */
export function EntityPreview({ entity, current }: { entity: Entity; current?: { kind: string; input?: string } }) {
  const { node } = entity;
  const line = node.kind === 'step' ? stepLine(node) : node.layer === 'code' ? node.id : node.summary;
  return (
    <div className={styles.preview}>
      <div className={styles.previewTop}>
        <PreviewHead node={node} label={node.layer === 'code' ? node.title : undefined} />
        {line !== '' && <p className={node.kind === 'step' || node.layer === 'code' ? styles.codeLine : styles.summary}>{line}</p>}
      </div>
      <KindSections entity={entity} />
      <ViewLinks views={entity.views} self={node.id} current={current} />
    </div>
  );
}
