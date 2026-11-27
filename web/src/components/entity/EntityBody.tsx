import { clsx } from 'clsx';
import { ArrowRight, ArrowUpRight, Code, FileCode } from 'lucide-react';
import type { CSSProperties, ReactNode } from 'react';
import { Link } from 'react-router';
import type { Entity, Neighbor, Node } from '../../api/types';
import { ABOUT, kindWord, MEMBER_COLUMNS, OP_COLUMNS, SECTIONS } from '../../terms';
import { ICON_SIZE, ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import table from '../../ui/table.module.css';
import { CaseSections, CheckedSections, CodeSections, ContainerSections, StepSections, TypeSections } from '../details/kindSections';
import { CodePanel, DiagnosticTag, NodeLink, NodeRow, Section, SourcePanel, ViaBadge } from '../details/sections';
import { Badge } from '../marks/Badge';
import { displayName, opLine, placeOf, relative, scopeOf, signatureParts, statusOf, stepLine, stepName } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { StatusDot } from '../marks/StatusDot';
import styles from './entity.module.css';

/*
 * The entity page (Figma 05, 06): a node in full on the canvas. Every kind has the common head (identity, status,
 * labels, summary and where it is written); a type and an operation have the prototype's own layout, and every other
 * kind lists the sections its preview shows.
 */

function Head({ entity }: { entity: Entity }) {
  const { node } = entity;
  const realized = entity.realizedBy[0]?.node;
  return (
    <header className={styles.head}>
      <div className={styles.identity}>
        <span className={styles.kindIcon}><KindIcon node={node} size={ICON_SIZE} /></span>
        <span className="t-mono-strong">{node.id}</span>
        <StatusDot status={statusOf(node)} />
        {(node.attrs.labels ?? []).map(label => <Badge key={label} tone="info">{label}</Badge>)}
        <span className={styles.spacer} />
        <InfoHint label={node.id}>{ABOUT.card(kindWord(node.kind, node.form, node.layer), node.form, `${node.origin.path}:${node.origin.line}`)}</InfoHint>
      </div>
      {node.summary !== '' && <h2 className={styles.summary}>{node.summary}</h2>}
      {node.kind === 'op' && node.layer === 'design' && <OpSignature op={node} />}
      <p className={styles.origin}>
        <span className={styles.place}><FileCode size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />{node.origin.path}:{node.origin.line}</span>
        {realized && (
          <NodeLink node={realized} className={styles.place} label={<><Code size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />{realized.origin.path}:{realized.origin.line}</>} />
        )}
      </p>
    </header>
  );
}

/** An operation's signature as its page shows it: the call in the link colour, then what it raises. */
function OpSignature({ op }: { op: Node }) {
  const { call, raises } = signatureParts(op);
  return <CodePanel lines={raises === undefined ? [<span key="call" className={styles.call}>{call}</span>] : [<span key="call" className={styles.call}>{call}</span>, `raises ${raises}`]} />;
}

/** The fields: the type's fields or values, the code's counterpart of each, and the status of the comparison. */
function MembersTable({ entity }: { entity: Entity }) {
  const links = useLinks();
  const members = entity.type?.members ?? [];
  const scope = scopeOf(entity.node.id);
  if (members.length === 0) {
    return null;
  }
  return (
    <Section title={members[0]?.kind === 'value' ? SECTIONS.values : SECTIONS.fields} about={ABOUT.fields}>
      <div className={clsx(table.table, styles.table)} role="table" style={{ '--columns': '8.5rem 13rem minmax(0, 1fr) 3rem' } as CSSProperties}>
        <div className={clsx(table.row, table.head, styles.headRow)} role="row">
          <span role="columnheader">{MEMBER_COLUMNS.name}</span>
          <span role="columnheader">{MEMBER_COLUMNS.type}</span>
          <span role="columnheader">{MEMBER_COLUMNS.code}</span>
          <span role="columnheader">{MEMBER_COLUMNS.status}</span>
        </div>
        {members.map(member => {
          const type = relative(member.type ?? '', scope);
          return (
            <div key={member.name} className={clsx(table.row, styles.bodyRow)} role="row" data-warn={member.status === 'contradicted'}>
              <span role="cell" className="t-code">{member.name}</span>
              <span role="cell" className="t-code">
                {member.refers.length > 0
                  ? <Link className={styles.typeLink} to={links.entityHref(member.refers[0] ?? '')}><ArrowUpRight size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />{type}</Link>
                  : type}
              </span>
              <span role="cell" className="t-code">{member.inCode ? `${member.name}: ${relative(member.codeType ?? '', scope)}` : ''}</span>
              <span role="cell"><StatusDot status={member.status} /></span>
            </div>
          );
        })}
      </div>
    </Section>
  );
}

/** The operations: each operation's signature, the functions that realize it and the properties about it. */
function OpsTable({ entity }: { entity: Entity }) {
  const ops = entity.type?.ops ?? [];
  if (ops.length === 0) {
    return null;
  }
  return (
    <Section title={SECTIONS.ops}>
      <div className={clsx(table.table, styles.table)} role="table" style={{ '--columns': 'minmax(0, 1.3fr) minmax(0, 1fr) minmax(0, 0.9fr)' } as CSSProperties}>
        <div className={clsx(table.row, table.head, styles.headRow)} role="row">
          <span role="columnheader">{OP_COLUMNS.signature}</span>
          <span role="columnheader">{OP_COLUMNS.realizedBy}</span>
          <span role="columnheader">{OP_COLUMNS.properties}</span>
        </div>
        {ops.map(row => (
          <div key={row.op.id} className={clsx(table.row, styles.bodyRow)} role="row">
            <span role="cell"><NodeLink node={row.op} label={opLine(row.op)} className={styles.plainLink} /></span>
            <span role="cell" className={styles.cellLinks}>
              {row.realizedBy.map(({ node }) => <NodeLink key={node.id} node={node} label={<><Code size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />{node.title} · {placeOf(node.origin)}</>} className={styles.iconLink} />)}
            </span>
            <span role="cell" className={styles.cellLinks}>
              {row.properties.map(property => <NodeLink key={property.id} node={property} label={<><KindIcon node={property} size={ICON_TINY} />{property.title}</>} className={styles.iconLink} />)}
            </span>
          </div>
        ))}
      </div>
    </Section>
  );
}

/** A diagram of one node between two columns of nodes, such as what refers to a type and what it refers to. */
function Between({ node, before, after, beforeTitle, afterTitle, badge }: {
  node: Node; before: Node[]; after: Neighbor[] | Node[]; beforeTitle: string; afterTitle: string; badge?: (neighbor: Neighbor) => ReactNode;
}) {
  const scope = scopeOf(node.id);
  const nodes = after.map(item => ('edge' in item ? item : undefined));
  return (
    <div className={styles.between}>
      <div className={styles.column}>
        <span className="t-micro c-faint">{beforeTitle}</span>
        {before.map(item => <NodeChip key={item.id} node={item} label={displayName(item, scope)} status={statusOf(item)} link />)}
      </div>
      <ArrowRight className={styles.arrow} size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
      <div className={styles.center}><NodeChip node={node} label={displayName(node, scope)} className={styles.self} /></div>
      <ArrowRight className={styles.arrow} size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
      <div className={styles.column}>
        <span className="t-micro c-faint">{afterTitle}</span>
        {after.map((item, index) => {
          const neighbor = nodes[index];
          const target = neighbor ? neighbor.node : (item as Node);
          return (
            <span key={target.id} className={styles.withBadge}>
              <NodeChip node={target} label={displayName(target, scope)} status={statusOf(target)} link />
              {neighbor && badge?.(neighbor)}
            </span>
          );
        })}
      </div>
    </div>
  );
}

/** The flows it takes part in: each step with what it does, as its line in the flow reads. */
function FlowSteps({ entity, children }: { entity: Entity; children?: (step: Node) => ReactNode }) {
  if (entity.flows.length === 0) {
    return null;
  }
  return (
    <Section title={SECTIONS.flows}>
      {entity.flows.map(step => (
        <div key={step.id} className={styles.flowRow}>
          <KindIcon node={{ kind: 'flow', form: 'flow', layer: 'design', salient: true }} size={ICON_SMALL} />
          <NodeLink node={step} label={stepName(step)} />
          <span className={styles.spacer} />
          <span className={styles.stepLine}>{stepLine(step)}</span>
          {children?.(step)}
        </div>
      ))}
    </Section>
  );
}

/** The code: the source of what realizes the node. */
function Sources({ entity }: { entity: Entity }) {
  if (entity.sources.length === 0) {
    return null;
  }
  return <Section title={SECTIONS.code} about={ABOUT.code}>{entity.sources.map(excerpt => <SourcePanel key={`${excerpt.path}:${excerpt.from}`} excerpt={excerpt} />)}</Section>;
}

function TypeBody({ entity }: { entity: Entity }) {
  const detail = entity.type;
  return (
    <>
      <MembersTable entity={entity} />
      <OpsTable entity={entity} />
      {detail && (detail.refers.length > 0 || detail.referredBy.length > 0) && (
        <Section title={SECTIONS.relations}>
          <Between node={entity.node} before={detail.referredBy} after={detail.refers} beforeTitle={SECTIONS.referredBy} afterTitle={SECTIONS.refers} />
        </Section>
      )}
      <TypeSections entity={entity} preview={false} />
      <FlowSteps entity={entity} />
      <Sources entity={entity} />
    </>
  );
}

function OpBody({ entity }: { entity: Entity }) {
  const detail = entity.op;
  const self = entity.realizedBy[0]?.node ?? entity.node;
  return (
    <>
      {detail && detail.signatureTypes.length > 0 && (
        <Section title={SECTIONS.signatureTypes}>
          <div className={styles.chips}>{detail.signatureTypes.map(node => <NodeChip key={node.id} node={node} label={displayName(node, scopeOf(entity.node.id))} status={statusOf(node)} link />)}</div>
        </Section>
      )}
      {detail && (detail.callers.length > 0 || detail.callees.length > 0) && (
        <Section title={SECTIONS.calls} about={ABOUT.calls}>
          <Between
            node={self} before={detail.callers.map(neighbor => neighbor.node)} after={detail.callees}
            beforeTitle={SECTIONS.caller} afterTitle={SECTIONS.callee}
            badge={neighbor => (neighbor.edge.kind === 'reaches' ? <ViaBadge count={neighbor.edge.attrs.via?.length ?? 0} /> : null)}
          />
        </Section>
      )}
      <FlowSteps entity={entity}>
        {step => (step.step && step.step.children.length > 0 ? <Badge tone="neutral">{SECTIONS.children} {step.step.children.map(id => `#${id.split('#').at(-1) ?? ''}`).join(' ')}</Badge> : null)}
      </FlowSteps>
      {entity.properties.length > 0 && (
        <Section title={SECTIONS.properties}>
          {entity.properties.map(property => (
            <div key={property.id} className={styles.property}>
              <NodeRow node={property} trailing={entity.diagnostics.filter(diagnostic => diagnostic.subject === property.id).map(diagnostic => <DiagnosticTag key={diagnostic.code} diagnostic={diagnostic} />)} />
              <p className="t-small c-muted">{property.summary}</p>
            </div>
          ))}
        </Section>
      )}
      <Sources entity={entity} />
    </>
  );
}

function OtherBody({ entity }: { entity: Entity }) {
  const { node } = entity;
  if (node.layer === 'code') {
    return <><CodeSections entity={entity} /><Sources entity={entity} /></>;
  }
  switch (node.kind) {
    case 'container':
    case 'actor':
      return <ContainerSections entity={entity} />;
    case 'step':
      return <StepSections entity={entity} />;
    case 'case':
      return <CaseSections entity={entity} />;
    case 'flow':
    case 'property':
      return <><CheckedSections entity={entity} /><FlowSteps entity={entity} /></>;
    case 'type':
    case 'op':
    case 'observation':
    case 'run':
    case 'result':
    case 'review':
      return null;
  }
}

/** The entity page's content on the canvas: the head, then the sections of the node's kind. */
export function EntityBody({ entity }: { entity: Entity }) {
  const { node } = entity;
  const design = node.layer === 'design';
  return (
    <article className={styles.page}>
      <Head entity={entity} />
      {design && node.kind === 'type' && <TypeBody entity={entity} />}
      {design && node.kind === 'op' && <OpBody entity={entity} />}
      {!(design && (node.kind === 'type' || node.kind === 'op')) && <OtherBody entity={entity} />}
    </article>
  );
}
