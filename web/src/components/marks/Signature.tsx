import type { Node } from '../../api/types';
import { signatureTypes, type TypePart } from './format';
import { NodeLink } from './NodeLink';
import { NodeRef } from './NodeRef';

/**
 * How the nodes a written type names act where it stands: as links to their pages, in a pane or on an entity page; or
 * as references that preview on a click, on a drawing.
 */
export type References = 'link' | 'preview';

/** A reference to a node in a written type, as the place it stands asks. */
function Reference({ id, label, references }: { id: string; label: string; references: References }) {
  return references === 'link' ? <NodeLink node={{ id, title: label }} tone="inherit" /> : <NodeRef id={id}>{label}</NodeRef>;
}

/** A written type, such as a field's type or a symbol's, with each node it names as a reference. */
export function TypeText({ parts, references }: { parts: TypePart[]; references: References }) {
  return parts.map((part, index) => (part.id === undefined ? part.text : <Reference key={index} id={part.id} label={part.text} references={references} />));
}

/**
 * An operation's signature with the operation and each type it names as references: its name and call, then what it
 * raises; the place it stands sets each part's class, as lines of a panel or a run of text.
 */
export function Signature({ op, references, callClassName, raisesClassName }: { op: Node; references: References; callClassName?: string; raisesClassName?: string }) {
  const { call, raises } = signatureTypes(op);
  return (
    <>
      <span className={callClassName}><Reference id={op.id} label={op.title} references={references} /><TypeText parts={call} references={references} /></span>
      {raises && <span className={raisesClassName}>raises <TypeText parts={raises} references={references} /></span>}
    </>
  );
}
