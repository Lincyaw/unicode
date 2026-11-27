import { Check, FileCode, FlaskConical, Play, Split, TriangleAlert, X } from 'lucide-react';
import type { Entity, Neighbor, Node, RunResult } from '../../api/types';
import { ABOUT, CASE_SOURCES, DIAGNOSTIC_WORDS, HISTORY, SECTIONS } from '../../terms';
import { ICON_SMALL, ICON_STROKE } from '../../ui/hint';
import { Badge } from '../marks/Badge';
import { fileOf, opLine, placeOf, relative, scopeOf, shortPath, stepLine, stepName, typeParts } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { caseSource } from './nodeWords';
import { useFolded } from './folded';
import { ChipList, CodePanel, DiagnosticTag, Facts, MoreBadge, NodeLink, NodeRow, Section, SignaturePanel, SymbolLine, TypeLinks, ViaBadge } from './sections';
import styles from './details.module.css';

/*
 * The sections of a node's page by its kind (api/entities.tsp), shared by the right pane's preview and the canvas's
 * entity page. Each takes the entity and shows only what it holds.
 */

/** How many chips or rows a preview lists before folding the rest into +n. */
const PREVIEW_LIMIT = 5;
const FILE_LIMIT = 4;

/** Name–value rows for the values a node has; an absent or empty value leaves its row out. */
function presentRows(pairs: [string, string | undefined][]): { label: string; value: string }[] {
  return pairs.flatMap(([label, value]) => (value === undefined || value === '' ? [] : [{ label, value }]));
}

/** The diagnostics of an entity whose subject is the given node or edge key. */
function diagnosticsOf(entity: Entity, subject: string): Entity['diagnostics'] {
  return entity.diagnostics.filter(diagnostic => diagnostic.subject === subject);
}

/** A code element's place, `service.py:40`, as the rows cite it on the right. */
function place(node: Node): string {
  return placeOf(node.origin);
}

/** The steps of each flow a node takes part in, grouped by flow. */
function flowsOf(steps: Node[]): { flow: Node; steps: Node[] }[] {
  const byFlow = Map.groupBy(steps, step => step.step?.flow ?? '');
  return [...byFlow].map(([flow, group]) => ({
    flow: { ...(group[0] as Node), id: flow, title: flow.split('.').at(-1) ?? flow, kind: 'flow', form: 'flow', status: undefined },
    steps: group,
  }));
}

/** The flows it takes part in: each flow with the steps the node takes part in. */
export function FlowsSection({ entity }: { entity: Entity }) {
  if (entity.flows.length === 0) {
    return null;
  }
  return (
    <Section title={SECTIONS.flows}>
      {flowsOf(entity.flows).map(({ flow, steps }) => (
        <NodeRow key={flow.id} node={flow} icon dot={false} tone="link" trailing={<span className={styles.indexes}>{steps.map(step => <NodeLink key={step.id} node={step} tone="inherit" />)}</span>} />
      ))}
    </Section>
  );
}

/** The related properties: each property with its status. */
export function PropertiesSection({ entity }: { entity: Entity }) {
  if (entity.properties.length === 0) {
    return null;
  }
  return (
    <Section title={SECTIONS.properties}>
      {entity.properties.map(property => <NodeRow key={property.id} node={property} icon tone="link" />)}
    </Section>
  );
}

/** What realizes it: the code elements that realize the node, with where they are. */
export function RealizedSection({ entity, title = SECTIONS.code }: { entity: Entity; title?: string }) {
  if (entity.realizedBy.length === 0) {
    return null;
  }
  return (
    <Section title={title}>
      <Facts rows={entity.realizedBy.map(({ node }) => ({ label: SECTIONS.realizedBy, value: <><NodeLink node={node} /> · {place(node)}</> }))} />
    </Section>
  );
}

/** A container's or an actor's sections: attributes, dependencies, contents, code. */
export function ContainerSections({ entity }: { entity: Entity }) {
  const { node } = entity;
  const detail = entity.container;
  const attributes = presentRows([
    [SECTIONS.focus, node.attrs.focus?.join(', ')],
    [SECTIONS.labels, node.attrs.labels?.join(', ')],
    [SECTIONS.paths, node.attrs.paths?.join(', ')],
  ]);
  if (node.kind === 'actor' && node.form === 'external') {
    return <ExternalSections entity={entity} />;
  }
  // A declared use and the actual dependency on the same component are one row.
  const declared = [...new Map((detail?.dependsOn ?? []).filter(({ edge }) => edge.status !== 'contradicted').map(neighbor => [neighbor.node.id, neighbor])).values()];
  const undeclared = [...(detail?.dependsOn ?? []), ...(detail?.dependedOnBy ?? [])].filter(({ edge }) => edge.status === 'contradicted');
  const files = detail?.files ?? [];
  return (
    <>
      {attributes.length > 0 && <Section title={SECTIONS.attributes}><Facts rows={attributes} /></Section>}
      {(declared.length > 0 || undeclared.length > 0) && (
        <Section title={SECTIONS.dependsOn}>
          {declared.map(({ node: other }) => <NodeRow key={other.id} node={other} />)}
          {undeclared.map(({ edge }) => (
            <div key={`${edge.from} ${edge.to}`} className={styles.row} data-tone="warning">
              <TriangleAlert size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
              <span className={styles.code}><NodeLink node={{ id: edge.from, title: edge.from }} tone="inherit" /> -&gt; <NodeLink node={{ id: edge.to, title: edge.to }} tone="inherit" /></span>
              <span className={styles.trailing}>
                <Badge tone="warning">{DIAGNOSTIC_WORDS.undeclared} {(edge.attrs.imports?.length ?? 0) + (edge.attrs.calls?.length ?? 0)}</Badge>
              </span>
            </div>
          ))}
        </Section>
      )}
      {detail && detail.contents.length > 0 && <Section title={SECTIONS.contents}><ChipList nodes={detail.contents} limit={PREVIEW_LIMIT} /></Section>}
      {files.length > 0 && <FilesSection files={files} />}
      <FlowsSection entity={entity} />
    </>
  );
}

/** A component's code files, past FILE_LIMIT a +n that unfolds the rest. */
function FilesSection({ files }: { files: Node[] }) {
  const { shown, more, unfold } = useFolded(files, FILE_LIMIT);
  return (
    <Section title={SECTIONS.code}>
      {shown.map(file => <NodeRow key={file.id} node={file} icon dot={false} tone="muted" label={shortPath(file.title)} />)}
      {more > 0 && <MoreBadge count={more} icon={FileCode} onClick={unfold} />}
    </Section>
  );
}

/** An external system's sections (Figma 12): what it holds, its operations' signatures, who depends on it, its clients. */
function ExternalSections({ entity }: { entity: Entity }) {
  const detail = entity.container;
  const ops = detail?.contents.filter(node => node.kind === 'op') ?? [];
  return (
    <>
      {detail && detail.contents.length > 0 && <Section title={SECTIONS.contents}><ChipList nodes={detail.contents} /></Section>}
      {ops.length > 0 && <Section title={SECTIONS.signature}>{ops.map(op => <SignaturePanel key={op.id} op={op} />)}</Section>}
      {detail && detail.dependedOnBy.length > 0 && (
        <Section title={SECTIONS.dependedOnBy}>
          {detail.dependedOnBy.map(({ node, edge }) => <NodeRow key={node.id} node={node} trailing={edge.kind} />)}
        </Section>
      )}
      <RealizedSection entity={entity} />
      <FlowsSection entity={entity} />
    </>
  );
}

/** A type's sections: attributes, fields, operations, references, events' publishers and subscribers. */
export function TypeSections({ entity, preview }: { entity: Entity; preview: boolean }) {
  const { node } = entity;
  const detail = entity.type;
  const scope = scopeOf(node.id);
  const container = entity.ancestors.at(-1);
  const attributes = presentRows([
    [SECTIONS.labels, node.attrs.labels?.join(', ')],
    [SECTIONS.container, container?.form === 'system' ? undefined : container?.title],
  ]);
  const members = detail?.members ?? [];
  return (
    <>
      {preview && attributes.length > 0 && <Section title={SECTIONS.attributes}><Facts rows={attributes} /></Section>}
      {preview && members.length > 0 && (
        <Section title={members[0]?.kind === 'value' ? SECTIONS.values : SECTIONS.fields}>
          {members.map(member => (
            <div key={member.name} className={styles.field} data-warn={member.status === 'contradicted' || member.status === 'missing'}>
              <span>{member.name}</span>
              <span className={styles.fieldType}><TypeLinks parts={typeParts(member.type ?? member.codeType ?? '', scope, member.refers)} /></span>
            </div>
          ))}
        </Section>
      )}
      {preview && detail && detail.ops.length > 0 && (
        <Section title={SECTIONS.ops}>
          {detail.ops.map(row => <NodeRow key={row.op.id} node={row.op} label={opLine(row.op)} />)}
        </Section>
      )}
      {detail && detail.emittedBy.length > 0 && (
        <Section title={SECTIONS.emittedBy}>
          {detail.emittedBy.map(step => (
            <div key={step.id} className={styles.stack}>
              <NodeRow node={step} label={<>{stepName(step)} · {step.step?.sender ? relative(step.step.sender.id, scope) : ''}</>} />
              {diagnosticsOf(entity, step.id).map(diagnostic => <DiagnosticTag key={diagnostic.code} diagnostic={diagnostic} />)}
            </div>
          ))}
        </Section>
      )}
      {detail && detail.subscribers.length > 0 && (
        <Section title={SECTIONS.subscribers}>{detail.subscribers.map(op => <NodeRow key={op.id} node={op} label={op.id} />)}</Section>
      )}
      {preview && detail && detail.refers.length > 0 && <Section title={SECTIONS.refers}><ChipList nodes={detail.refers} scope={scope} /></Section>}
      {preview && detail && detail.referredBy.length > 0 && <Section title={SECTIONS.referredBy}><ChipList nodes={detail.referredBy} scope={scope} /></Section>}
      {preview && <PropertiesSection entity={entity} />}
      {preview && <FlowsSection entity={entity} />}
      {preview && <RealizedSection entity={entity} />}
    </>
  );
}

/** A design operation's sections in the preview: signature, callers and callees, properties, flows, code. */
export function OpSections({ entity }: { entity: Entity }) {
  const callers = entity.op?.callers ?? [];
  const callees = entity.op?.callees ?? [];
  return (
    <>
      <Section title={SECTIONS.signature}><SignaturePanel op={entity.node} /></Section>
      {(callers.length > 0 || callees.length > 0) && (
        <Section title={SECTIONS.calls}>
          {callers.map(({ node }) => <NodeRow key={node.id} node={node} icon dot={false} trailing={place(node)} />)}
          {callees.map(neighbor => <CalleeRow key={neighbor.node.id} neighbor={neighbor} />)}
        </Section>
      )}
      <PropertiesSection entity={entity} />
      <FlowsSection entity={entity} />
      <RealizedSection entity={entity} title={SECTIONS.realizedBy} />
    </>
  );
}

/** A callee: its name, and where it is or how many detail functions lead to it. */
function CalleeRow({ neighbor }: { neighbor: Neighbor }) {
  const via = neighbor.edge.kind === 'reaches' ? neighbor.edge.attrs.via?.length ?? 0 : 0;
  return <NodeRow node={neighbor.node} icon dot={false} trailing={via > 0 ? <ViaBadge count={via} /> : place(neighbor.node)} />;
}

/** A step's sections (Figma 02, 09): what it calls or handles, who calls whom, its code trace, sub-steps and cases. */
export function StepSections({ entity }: { entity: Entity }) {
  const { node } = entity;
  const info = node.step;
  const detail = entity.step;
  const target = info?.target;
  const evidence = entity.evidence;
  const links = useLinks();
  return (
    <>
      {node.form === 'on' && target && <Section title={SECTIONS.handles}><ChipList nodes={[target]} /></Section>}
      {node.form === 'on' && detail?.call && (
        <Section title={SECTIONS.call}>
          <NodeRow node={detail.call} label={`${detail.call.title} ${stepLine(detail.call).replace(/^.* -> /, '')}`} />
          {detail.call.step?.target && <SignaturePanel op={detail.call.step.target} />}
        </Section>
      )}
      {node.form !== 'on' && target?.kind === 'op' && <Section title={SECTIONS.signature}><SignaturePanel op={target} /></Section>}
      {(info?.sender || info?.receiver) && (
        <Section title={SECTIONS.calls}>
          <Facts rows={[
            ...(info.sender ? [{ label: SECTIONS.caller, value: <NodeLink node={info.sender} label={info.sender.id} tone="main" /> }] : []),
            ...(info.receiver ? [{ label: SECTIONS.callee, value: <NodeLink node={info.receiver} label={info.receiver.id} tone="main" /> }] : []),
            ...((node.attrs.args ?? []).length > 0 ? [{ label: SECTIONS.args, value: (node.attrs.args ?? []).join(', ') }] : []),
          ]} />
        </Section>
      )}
      {detail && (detail.traces.length > 0 || detail.callSites.length > 0) && (
        <Section title={SECTIONS.trace}>
          <Facts rows={[
            ...detail.traces.map(({ node: fn }) => ({ label: SECTIONS.realizedBy, value: <><NodeLink node={fn} /> · {place(fn)}</> })),
            ...detail.callSites.map(({ node: fn, edge }) => ({ label: SECTIONS.callSites, value: <><NodeLink node={fn} /> · {edge.attrs.sites?.map(site => placeOf({ path: site.split(':')[0] ?? '', line: Number(site.split(':')[1]) })).join(', ')}</> })),
          ]} />
        </Section>
      )}
      {detail && detail.children.length > 0 && (
        <Section title={SECTIONS.children}>
          {detail.children.map(child => <NodeRow key={child.id} node={child} label={`${child.title} ${stepLine(child).replace(/^[^ ]+ -> /, '')}`} />)}
        </Section>
      )}
      {(evidence?.branches !== undefined || (entity.coveredBy?.length ?? 0) > 0) && (
        <Section title={SECTIONS.stepCases} about={ABOUT.stepCases}>
          <div className={styles.chips}>
            {evidence?.branches?.map(branch => <Badge key={branch.label} tone="neutral" icon={Split}>{branch.label} {branch.cases}</Badge>)}
            {entity.coveredBy?.map(item => <Badge key={item.id} tone="info" icon={FlaskConical} to={links.entityHref(item.id)}>{item.title}</Badge>)}
          </div>
        </Section>
      )}
      <PropertiesSection entity={entity} />
    </>
  );
}

/** What a case's result in one run was, read along its history from the oldest. */
function historyWords(history: RunResult[]): string[] {
  let failedBefore = false;
  return history.map(result => {
    if (result.outcome === 'failed') {
      const word = failedBefore ? HISTORY.failed : HISTORY.firstFailure;
      failedBefore = true;
      return word;
    }
    if (result.mode === 'replay' && !failedBefore) {
      return HISTORY.replayPassed;
    }
    const word = failedBefore ? HISTORY.fixed : HISTORY.passed;
    failedBefore = false;
    return word;
  });
}

/** A case's sections (Figma 03): its source, values, history and the functions it covers. */
export function CaseSections({ entity }: { entity: Entity }) {
  const { node } = entity;
  const source = caseSource(node);
  const history = [...(entity.evidence?.history ?? [])].reverse();
  const words = historyWords(history);
  return (
    <>
      <div className={styles.chips}>
        <Badge tone={source === CASE_SOURCES.counterexample ? 'warning' : source === CASE_SOURCES.recorded ? 'info' : 'neutral'} icon={source === CASE_SOURCES.counterexample ? TriangleAlert : undefined}>{source}</Badge>
      </div>
      {(node.attrs.values ?? []).length > 0 && <Section title={SECTIONS.valuesOfCase}><CodePanel lines={node.attrs.values ?? []} /></Section>}
      {history.length > 0 && (
        <Section title={SECTIONS.history} about={ABOUT.history}>
          {history.map((result, index) => {
            const replay = result.mode === 'replay' && index === history.length - 1;
            const Icon = result.outcome === 'failed' ? X : replay ? Play : Check;
            return (
              <div key={`${result.commit}${result.at}`} className={styles.row} data-tone={result.outcome === 'failed' ? 'warning' : replay ? undefined : 'success'}>
                <Icon size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
                <span className={styles.code}>{replay ? result.at.slice(11, 16) : result.commit.slice(0, 7)}</span>
                <span className={styles.trailing}>{words[index]}</span>
              </div>
            );
          })}
        </Section>
      )}
      {entity.case && entity.case.covers.length > 0 && (
        <Section title={SECTIONS.covers} about={ABOUT.covers}>
          {entity.case.covers.map(({ node: fn, edge }) => <NodeRow key={fn.id} node={fn} icon dot={false} trailing={`${placeOf({ path: fn.origin.path, line: Number(edge.attrs.lines?.[0] ?? fn.origin.line) })}`} />)}
        </Section>
      )}
    </>
  );
}

/** A flow's or a property's sections: inputs, predicate and cases. */
export function CheckedSections({ entity }: { entity: Entity }) {
  const { node } = entity;
  const checked = entity.flow ?? entity.property;
  const inputs = [...(node.attrs.symbols ?? []).map(symbol => <SymbolLine key={symbol} symbol={symbol} />), ...(node.attrs.limits ?? []).map(limit => `  where ${limit}`)];
  return (
    <>
      {entity.property && entity.property.about.length > 0 && <Section title={SECTIONS.about}><ChipList nodes={entity.property.about} /></Section>}
      {inputs.length > 0 && <Section title={SECTIONS.inputs} about={ABOUT.inputs}><CodePanel lines={inputs} /></Section>}
      {checked?.predicate && (
        <Section title={SECTIONS.predicate}>
          <NodeRow node={checked.predicate.node} icon trailing={place(checked.predicate.node)} />
        </Section>
      )}
      {checked && checked.cases.length > 0 && (
        <Section title={SECTIONS.cases}>
          {checked.cases.map(row => <NodeRow key={row.node.id} node={row.node} icon trailing={caseSource(row.node)} />)}
        </Section>
      )}
      {entity.flow && entity.flow.steps.length > 0 && (
        <Section title={SECTIONS.stepsOfFlow}>
          {entity.flow.steps.map(step => <NodeRow key={step.id} node={step} label={`${step.title} ${stepLine(step)}`} />)}
        </Section>
      )}
    </>
  );
}

/** A code element's sections (Figma 11): what it realizes, where it is, its declaration, callers, callees and details. */
export function CodeSections({ entity }: { entity: Entity }) {
  const links = useLinks();
  const { node } = entity;
  const detail = entity.code;
  const bindings = node.bindings ?? [];
  return (
    <>
      {bindings.length > 0 && (
        <Section title={SECTIONS.realizedBy}>
          {bindings.map(({ node: bound }) => <NodeRow key={bound.id} node={bound} icon label={bound.id} />)}
        </Section>
      )}
      <Section title={SECTIONS.location}>
        <Facts rows={[{ label: SECTIONS.file, value: <NodeLink node={{ id: fileOf(node.origin.path), title: node.origin.path }} tone="main" /> }, { label: SECTIONS.line, value: String(node.origin.line) }]} />
      </Section>
      {detail?.declaration !== undefined && <Section title={SECTIONS.signature}><CodePanel lines={[detail.declaration]} /></Section>}
      {detail?.owner && <Section title={SECTIONS.owner}><ChipList nodes={[detail.owner]} /></Section>}
      {detail && detail.contents.length > 0 && <Section title={SECTIONS.contents}><ChipList nodes={detail.contents} /></Section>}
      {detail && detail.callers.length > 0 && (
        <Section title={SECTIONS.caller}>
          {detail.callers.map(({ node: caller }) => <NodeRow key={caller.id} node={caller} icon dot={false} trailing={place(caller)} />)}
        </Section>
      )}
      {detail && detail.callees.length > 0 && (
        <Section title={SECTIONS.callee}>
          {detail.callees.map(neighbor => <CalleeRow key={neighbor.node.id} neighbor={neighbor} />)}
        </Section>
      )}
      {detail && detail.details.length > 0 && (
        <Section title={SECTIONS.details} about={ABOUT.details}>
          <div className={styles.chips}>
            {detail.details.map(fn => <Badge key={fn.id} tone="neutral" mono to={links.entityHref(fn.id)}><KindIcon node={fn} size={12} />{fn.title}</Badge>)}
          </div>
        </Section>
      )}
    </>
  );
}
