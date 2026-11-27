import { clsx } from 'clsx';
import type { CSSProperties } from 'react';
import type { DriverMode, View, ViewNode } from '../../api/types';
import { ABOUT, ADEQUACY, CASE_COLUMNS, CASE_SOURCES, CONTROLS, DRIVER_MODES, formWord, OUTCOMES, SECTIONS } from '../../terms';
import { Tag } from '../../ui/content';
import { MarkButton } from '../../ui/controls';
import { Icon } from '../../ui/icons';
import table from '../../ui/table.module.css';
import { caseSource } from '../details/nodeWords';
import { CodePanel, Section, SymbolLine } from '../details/sections';
import { placeOf, relative, scopeOf, statusOf } from '../marks/format';
import { Dices, FileCode, KindIcon, Play, Repeat, TriangleAlert, type LucideIcon } from '../marks/icons';
import { useLinks, useNodeActions } from '../marks/links';
import { NodeRef } from '../marks/NodeRef';
import { Signature } from '../marks/Signature';
import { NodeChip } from '../marks/NodeChip';
import { StatusDot } from '../marks/StatusDot';
import styles from './property.module.css';

const DRIVER_ICONS: Record<DriverMode, LucideIcon> = { replay: Play, explore: Dices, fuzz: Repeat };

/** The predicate's signature as its row reads: its name and its parameters, relative to the property's component. */
function predicateLine(predicate: ViewNode, scope: string): string {
  return `${predicate.title}${relative(predicate.attrs.signature?.[0] ?? '()', scope)}`;
}

/** An operation the property constrains, as its line reads: its name and signature, the operation and each type a reference. */
function AboutSignature({ op }: { op: ViewNode }) {
  return <span className={styles.signature}><Signature op={op} references="preview" /></span>;
}

/** A case's name in the case table, which previews the case. */
function CaseButton({ item }: { item: ViewNode }) {
  return <MarkButton className={styles.code} {...useNodeActions(item.id)}>{item.title}</MarkButton>;
}

function Adequacy({ label, value }: { label: string; value?: number }) {
  return (
    <div className={styles.meter}>
      <span className={styles.meterHead}>
        <span>{label}</span>
        <span className="t-numeric">{value === undefined ? CONTROLS.none : `${Math.round(value * 100)}%`}</span>
      </span>
      <span className={styles.track}><span className={styles.fill} style={{ '--share': value ?? 0 } as CSSProperties} /></span>
    </div>
  );
}

function PropertyBlock({ property, view }: { property: ViewNode; view: View }) {
  const links = useLinks();
  const scope = scopeOf(property.id);
  const byId = new Map(view.nodes.map(node => [node.id, node]));
  const about = view.edges.filter(edge => edge.kind === 'about' && edge.from === property.id).flatMap(edge => byId.get(edge.to) ?? []);
  const cases = view.edges.filter(edge => edge.kind === 'contains' && edge.from === property.id).flatMap(edge => byId.get(edge.to) ?? []).filter(node => node.kind === 'case');
  const predicate = view.edges.filter(edge => edge.kind === 'checks' && edge.to === property.id).flatMap(edge => byId.get(edge.from) ?? [])[0];
  const evidence = property.evidence;
  const inputs = [...(property.attrs.symbols ?? []).map(symbol => <SymbolLine key={symbol} symbol={symbol} scope={scope} refs={property.refers} />), ...(property.attrs.limits ?? []).map(limit => `  where ${limit}`)];
  return (
    <article className={styles.block}>
      <div className={styles.card}>
        <div className={styles.cardHead}>
          <StatusDot status={property.status} />
          <KindIcon node={property} />
          <MarkButton className={styles.identity} {...useNodeActions(property.id)}>{property.id}</MarkButton>
          <span className="t-small c-muted">{formWord(property.kind, property.form)}</span>
        </div>
        <p className={styles.statement}>{property.summary}</p>
        {about.map(node => (
          <div key={node.id} className={styles.about}>
            <span className="t-small c-muted">{SECTIONS.about}</span>
            <NodeChip node={node} label={node.id.split('.').slice(1).join('.')} />
            {node.kind === 'op' && <AboutSignature op={node} />}
          </div>
        ))}
      </div>
      {inputs.length > 0 && <Section title={SECTIONS.inputs} about={ABOUT.inputs}><CodePanel lines={inputs} /></Section>}
      <Section title={SECTIONS.predicate} about={ABOUT.predicate}>
        {predicate
          ? (
            <div className={styles.predicate}>
              <StatusDot status={statusOf(predicate)} />
              <NodeRef id={predicate.id} className={styles.code}>{predicateLine(predicate, scope)}</NodeRef>
              <span className={styles.place}><Icon glyph={FileCode} size="tiny" />{placeOf(predicate.origin)}</span>
            </div>
          )
          : <span className={styles.absent}>{CONTROLS.none}</span>}
        {(evidence?.drivers ?? []).length > 0 && (
          <div className={styles.drivers}>
            {evidence?.drivers?.map(driver => (
              <Tag key={driver.mode} tone={driver.mode === 'replay' ? 'success' : 'neutral'}>
                <Icon glyph={DRIVER_ICONS[driver.mode]} size="tiny" />
                {DRIVER_MODES[driver.mode]} · {driver.mode === 'replay' ? driver.cases : driver.tool}
              </Tag>
            ))}
          </div>
        )}
      </Section>
      {cases.length > 0 && (
        <Section title={SECTIONS.cases} about={ABOUT.cases}>
          <div className={clsx(table.table, styles.table)} role="table" style={{ '--columns': '8rem 4.5rem minmax(0, 1fr) 4.5rem 4.5rem' } as CSSProperties}>
            <div className={clsx(table.row, table.head, styles.headRow)} role="row">
              {[CASE_COLUMNS.name, CASE_COLUMNS.source, CASE_COLUMNS.values, CASE_COLUMNS.outcome, CASE_COLUMNS.gain].map(column => <span key={column} role="columnheader">{column}</span>)}
            </div>
            {cases.map(item => {
              const source = caseSource(item);
              const outcome = item.evidence?.outcome;
              const gain = item.evidence?.coverageGain;
              return (
                <div key={item.id} className={clsx(table.row, styles.bodyRow)} role="row" aria-selected={links.selected === item.id}>
                  <span role="cell"><CaseButton item={item} /></span>
                  <span role="cell">{source === CASE_SOURCES.counterexample ? <Tag tone="warning"><Icon glyph={TriangleAlert} size="tiny" />{source}</Tag> : <Tag>{source}</Tag>}</span>
                  <span role="cell" className={styles.values}>{(item.attrs.values ?? []).map(value => value.replace(/^\w+ = /, '')).join(', ')}</span>
                  <span role="cell" className={styles.outcome} data-outcome={outcome}>{outcome && <><StatusDot status={outcome === 'passed' ? 'verified' : 'contradicted'} />{OUTCOMES[outcome]}</>}</span>
                  <span role="cell" className={styles.gain}>{gain === undefined ? CONTROLS.none : `+${gain} ${CONTROLS.linesUnit}`}</span>
                </div>
              );
            })}
          </div>
        </Section>
      )}
      {evidence?.adequacy && (
        <Section title={SECTIONS.adequacy} about={ABOUT.adequacy}>
          <div className={styles.meters}>
            <Adequacy label={ADEQUACY.code} value={evidence.adequacy.code} />
            <Adequacy label={ADEQUACY.branch} value={evidence.adequacy.branch} />
            <Adequacy label={ADEQUACY.error} value={evidence.adequacy.error} />
          </div>
        </Section>
      )}
    </article>
  );
}

/**
 * The property view (Figma 03): a property's statement and what it constrains, its inputs, its predicate and drivers,
 * its cases with their results, and its test adequacy; a component's view lists each of its properties so.
 */
export function PropertyView({ view }: { view: View }) {
  const properties = view.nodes.filter(node => node.kind === 'property');
  return (
    <div className={styles.scroll}>
      {properties.map(property => <PropertyBlock key={property.id} property={property} view={view} />)}
    </div>
  );
}
