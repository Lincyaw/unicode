import { clsx } from 'clsx';
import { ArrowUpRight, Ellipsis, TriangleAlert, type LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { requestedView } from '../../routes/address';
import type { CodeExcerpt, Diagnostic, Node, ViewRequest } from '../../api/types';
import { ABOUT, CONTROLS, kindWord, SECTIONS, VIEW_NAMES } from '../../terms';
import { ICON_SIZE, ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import { Badge } from '../marks/Badge';
import { displayName, signatureTypes, statusOf, typeParts, type TypePart } from '../marks/format';
import { KindIcon } from '../marks/icons';
import { useLinks } from '../marks/links';
import { NodeChip } from '../marks/NodeChip';
import { StatusDot } from '../marks/StatusDot';
import { useFolded } from './folded';
import styles from './details.module.css';

/*
 * The parts the right pane and the entity pages are made of: a section with its head, rows of facts, rows and chips of
 * linked nodes, panels of code, and the links to related views. Every reference is a link to its node's page.
 */

/** A section: a small head, with its ⓘ when there is something to explain, over its content. */
export function Section({ title, about, children, className }: { title: string; about?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={clsx(styles.section, className)}>
      <h3 className={styles.sectionHead}>
        {title}
        {about !== undefined && <InfoHint label={title}>{about}</InfoHint>}
      </h3>
      {children}
    </section>
  );
}

/** Name–value rows; values are set in the code face. */
export function Facts({ rows }: { rows: { label: string; value: ReactNode }[] }) {
  return (
    <dl className={styles.facts}>
      {rows.map((row, index) => (
        <div key={`${index} ${row.label}`}>
          <dt>{row.label}</dt>
          <dd>{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** How a link to a node reads: in the text colour, as a link, muted like a path, or as the text around it. */
export type LinkTone = 'main' | 'link' | 'muted' | 'inherit';

/** A link to a node's page, set in the code face. */
export function NodeLink({ node, label, tone = 'link', className }: { node: Pick<Node, 'id' | 'title'>; label?: ReactNode; tone?: LinkTone; className?: string }) {
  const links = useLinks();
  return <Link className={clsx(styles.nodeLink, className)} data-tone={tone} to={links.entityHref(node.id)}>{label ?? node.title}</Link>;
}

/** One node in a list: its status, optionally its kind, the link to it, and what follows it on the right. */
export function NodeRow({ node, label, icon = false, dot = true, tone = 'main', trailing }: {
  node: Node; label?: ReactNode; icon?: boolean; dot?: boolean; tone?: LinkTone; trailing?: ReactNode;
}) {
  return (
    <div className={styles.row}>
      {dot && <StatusDot status={statusOf(node)} />}
      {icon && <KindIcon node={node} size={ICON_SMALL} />}
      <NodeLink node={node} label={label} tone={tone} />
      {trailing !== undefined && <span className={styles.trailing}>{trailing}</span>}
    </div>
  );
}

/** A written type with each node it names as a link to that node's page, in the face of the text around it. */
export function TypeLinks({ parts }: { parts: TypePart[] }) {
  return parts.map((part, index) => (part.id === undefined ? part.text : <NodeLink key={index} node={{ id: part.id, title: part.text }} tone="inherit" />));
}

/** A property's symbol as its inputs read it, `forall name: type`, each type it names a link to its page. */
export function SymbolLine({ symbol, scope }: { symbol: string; scope?: string }) {
  const at = symbol.indexOf(':');
  return at < 0 ? <>forall {symbol}</> : <>forall {symbol.slice(0, at + 1)}<TypeLinks parts={typeParts(symbol.slice(at + 1), scope)} /></>;
}

/** The +n that unfolds a list in place. */
export function MoreBadge({ count, icon, onClick }: { count: number; icon?: LucideIcon; onClick: () => void }) {
  return <Badge tone="neutral" icon={icon} label={CONTROLS.expandRow} onClick={onClick}>+{count}</Badge>;
}

/** Nodes as chips that link to their pages, named relative to `scope`; past `limit`, a +n that unfolds the rest. */
export function ChipList({ nodes, limit, scope }: { nodes: Node[]; limit?: number; scope?: string }) {
  const { shown, more, unfold } = useFolded(nodes, limit);
  return (
    <div className={styles.chips}>
      {shown.map(node => <NodeChip key={node.id} node={node} label={displayName(node, scope)} status={statusOf(node)} link />)}
      {more > 0 && <MoreBadge count={more} onClick={unfold} />}
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

/** An operation's signature as a panel: the call and result, then what it raises; the operation and each type a link to its page. */
export function SignaturePanel({ op }: { op: Node }) {
  const { call, raises } = signatureTypes(op);
  const line = <><NodeLink node={op} tone="inherit" /><TypeLinks parts={call} /></>;
  return <CodePanel lines={raises === undefined ? [line] : [line, <span key="raises" className="c-muted">raises <TypeLinks parts={raises} /></span>]} />;
}

/** Lines of a file, as the code section shows them. */
export function SourcePanel({ excerpt }: { excerpt: CodeExcerpt }) {
  return <CodePanel className={styles.source} lines={excerpt.text.replace(/\n$/, '').split('\n')} />;
}

/** A diagnostic as a tag: its code, with its cause and fix behind the ⓘ. */
export function DiagnosticTag({ diagnostic }: { diagnostic: Diagnostic }) {
  return (
    <span className={styles.diagnostic}>
      <Badge tone="warning" icon={TriangleAlert}>{diagnostic.code}</Badge>
      <InfoHint label={diagnostic.code}>
        <span>{diagnostic.message}</span>
        <span>{SECTIONS.fix}{CONTROLS.colon}{diagnostic.fix}</span>
      </InfoHint>
    </span>
  );
}

/** The name of a related view: the view, and its input when that is not the node the page is about. */
function viewLabel(request: ViewRequest, self: string): string {
  return request.input.id === self || request.input.form === 'system' ? VIEW_NAMES[request.kind] : `${VIEW_NAMES[request.kind]} ${request.input.title}`;
}

/** The related views, each a link once, leaving out the view the page already shows. */
export function ViewLinks({ views, self, current }: { views: ViewRequest[]; self: string; current?: { kind: string; input?: string } }) {
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
        {shown.map(request => <Badge key={`${request.kind} ${request.input.id}`} tone="info" icon={ArrowUpRight} to={links.viewHref(request)}>{viewLabel(request, self)}</Badge>)}
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
      {icon ?? <KindIcon node={node} size={ICON_SIZE} />}
      <Link className={clsx(styles.identity, styles.identityLink)} to={links.entityHref(node.id)}>{label ?? node.id}</Link>
      <InfoHint label={node.id}>{ABOUT.card(kindWord(node.kind, node.form, node.layer), node.form, `${node.origin.path}:${node.origin.line}`)}</InfoHint>
      <Link className={styles.enter} to={links.entityHref(node.id)} aria-label={CONTROLS.open}>
        <ArrowUpRight size={ICON_SIZE} strokeWidth={ICON_STROKE} aria-hidden="true" />
      </Link>
    </div>
  );
}

/** A count of detail functions folded into an edge, as an edge's badge and a callee's row show it. */
export function ViaBadge({ count }: { count: number }) {
  return <Badge tone="info" icon={Ellipsis}>{count}</Badge>;
}

/** The small icon of a node beside text, at the tag size. */
export function TinyIcon({ node }: { node: Node }) {
  return <KindIcon node={node} size={ICON_TINY} />;
}
