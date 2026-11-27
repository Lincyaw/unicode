import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import type { CodeExcerpt, Diagnostic, Node, ViewRequest } from '../../api/types';
import { requestedView } from '../../routes/address';
import { ABOUT, CONTROLS, SECTIONS, VIEW_NAMES } from '../../terms';
import { FactList, HintedHeading, Tag, type Fact } from '../../ui/content';
import { Button, ButtonLink, IconButton, LinkArrowIcon } from '../../ui/controls';
import { InfoHint } from '../../ui/hint';
import { Icon } from '../../ui/icons';
import { displayName, statusOf, typeParts } from '../marks/format';
import { ArrowUpRight, Ellipsis, KindIcon, TriangleAlert, type LucideIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { NodeLink, type LinkTone } from '../marks/NodeLink';
import { Signature, TypeText } from '../marks/Signature';
import { StatusDot } from '../marks/StatusDot';
import { ViewName } from '../marks/ViewName';
import { useFolded } from './folded';
import styles from './details.module.css';

/*
 * The parts the right pane and the entity pages are made of: a section with its head, rows of facts, rows and chips of
 * linked nodes, panels of code, and the links to related views. Every reference is a link to its node's page.
 */

/** A section: the kit's small hinted heading, with its ⓘ when there is something to explain, over its content. */
export function Section({ title, about, children, className }: { title: string; about?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={clsx(styles.section, className)}>
      <HintedHeading rank="h3" className="t-micro c-muted" title={title} about={about} />
      {children}
    </section>
  );
}

/** Name–value pairs, as the kit's fact list lays them out in a pane; the values are code: identities, paths and lists. */
export function Facts({ rows }: { rows: { label: string; value: ReactNode }[] }) {
  return <FactList layout="pairs" facts={rows.map((row): Fact => ({ ...row, format: 'code' }))} />;
}

/** One node in a list: its status, optionally its kind, the link to it, and what follows it on the right. */
export function NodeRow({ node, label, icon = false, dot = true, tone = 'main', trailing }: {
  node: Node; label?: ReactNode; icon?: boolean; dot?: boolean; tone?: LinkTone; trailing?: ReactNode;
}) {
  return (
    <div className={styles.row}>
      {dot && <StatusDot status={statusOf(node)} />}
      {icon && <KindIcon node={node} size="small" />}
      <NodeLink node={node} label={label} tone={tone} />
      {trailing !== undefined && <span className={styles.trailing}>{trailing}</span>}
    </div>
  );
}

/**
 * A property's symbol as its inputs read it, `forall name: type`, each type it names a link to its page; `refs` are
 * the property's refers, which say what the type names.
 */
export function SymbolLine({ symbol, scope, refs }: { symbol: string; scope?: string; refs: readonly string[] }) {
  const at = symbol.indexOf(':');
  return at < 0 ? <>forall {symbol}</> : <>forall {symbol.slice(0, at + 1)}<TypeText parts={typeParts(symbol.slice(at + 1), scope, refs)} references="link" /></>;
}

/** The +n that unfolds a list in place, a quiet small pill. */
export function UnfoldButton({ count, icon, onClick }: { count: number; icon?: LucideIcon; onClick: () => void }) {
  return <Button variant="quiet" size="small" onClick={onClick}>{icon && <Icon glyph={icon} size="tiny" />}+{count}</Button>;
}

/** Nodes as chips that link to their pages, named relative to `scope`; past `limit`, a +n that unfolds the rest. */
export function ChipList({ nodes, limit, scope }: { nodes: Node[]; limit?: number; scope?: string }) {
  const { shown, more, unfold } = useFolded(nodes, limit);
  return (
    <div className={styles.chips}>
      {shown.map(node => <NodeChip key={node.id} node={node} label={displayName(node, scope)} status={statusOf(node)} mode="link" />)}
      {more > 0 && <UnfoldButton count={more} onClick={unfold} />}
    </div>
  );
}

/** A panel of code or values, line by line. */
export function CodePanel({ lines, className }: { lines: ReactNode[]; className?: string }) {
  return (
    <pre className={clsx(styles.panel, className)}>
      {lines.map((line, index) => <span key={index} className={styles.panelLine}>{line}</span>)}
    </pre>
  );
}

/**
 * An operation's signature as a panel: the call and result, then what it raises; the operation and each type a link to
 * its page. `callClassName` lets a page set its call apart, as the entity page does in the link colour.
 */
export function SignaturePanel({ op, callClassName }: { op: Node; callClassName?: string }) {
  return <pre className={styles.panel}><Signature op={op} references="link" callClassName={clsx(styles.panelLine, callClassName)} raisesClassName={clsx(styles.panelLine, 'c-muted')} /></pre>;
}

/** Lines of a file, as the code section shows them. */
export function SourcePanel({ excerpt }: { excerpt: CodeExcerpt }) {
  return <CodePanel className={styles.source} lines={excerpt.text.replace(/\n$/, '').split('\n')} />;
}

/** A diagnostic as a tag: its code, with its cause and fix behind the ⓘ. */
export function DiagnosticTag({ diagnostic }: { diagnostic: Diagnostic }) {
  return (
    <span className={styles.diagnostic}>
      <Tag tone="warning"><Icon glyph={TriangleAlert} size="tiny" />{diagnostic.code}</Tag>
      <InfoHint subject={diagnostic.code}>
        <span>{diagnostic.message}</span>
        <span>{SECTIONS.fix}{CONTROLS.colon}{diagnostic.fix}</span>
      </InfoHint>
    </span>
  );
}

/** The related views, each a small pill that names the view and its input, once each, leaving out the view the page already shows. */
export function ViewLinks({ views, current }: { views: ViewRequest[]; current?: { kind: string; input?: string } }) {
  const links = useLinks();
  const seen = new Set<string>();
  const shown = views.filter(request => {
    const key = `${request.kind} ${request.input.id}`;
    const isCurrent = current !== undefined && request.kind === current.kind && current.input === requestedView(request, undefined).input;
    if (seen.has(key) || isCurrent) {
      return false;
    }
    seen.add(key);
    return true;
  });
  if (shown.length === 0) {
    return null;
  }
  return (
    <Section title={SECTIONS.views}>
      <div className={styles.chips}>
        {shown.map(request => (
          <ButtonLink key={`${request.kind} ${request.input.id}`} to={links.viewHref(request)} variant="line" size="small">
            <Icon glyph={ArrowUpRight} size="tiny" /><ViewName view={VIEW_NAMES[request.kind]} input={request.input.title} />
          </ButtonLink>
        ))}
      </div>
    </Section>
  );
}

/** The head of a preview: status, kind, identity, its ⓘ and the ↗ into its page. */
export function PreviewHead({ node, icon, label }: { node: Node; icon?: ReactNode; label?: string }) {
  const links = useLinks();
  return (
    <div className={styles.previewHead}>
      <StatusDot status={statusOf(node)} />
      {icon ?? <KindIcon node={node} />}
      <span className={styles.identity}>{label ?? node.id}</span>
      <InfoHint subject={node.id}>{ABOUT.card(node, `${node.origin.path}:${node.origin.line}`)}</InfoHint>
      <IconButton label={CONTROLS.open} to={links.entityHref(node.id)} variant="quiet" size="small"><LinkArrowIcon /></IconButton>
    </div>
  );
}

/** A count of detail functions folded into an edge, as a tag on the edge and on a callee's row. */
export function ViaTag({ count }: { count: number }) {
  return <Tag><Icon glyph={Ellipsis} size="tiny" />{count}</Tag>;
}

/** The small icon of a node beside text, at the tag size. */
export function TinyIcon({ node }: { node: Node }) {
  return <KindIcon node={node} size="tiny" />;
}
