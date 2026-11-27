import { REVIEW_DECISIONS } from '../../terms';
import type { Entity, Node, Review, StepInfo } from '../../api/types';

/*
 * How identities and signatures read on a page. The graph writes every reference from the root (docs/design.md §5.2);
 * a page drops the prefix of the component the reader is in, so `order.Cart` reads `Cart` inside order and
 * `inventory.OutOfStock` keeps its component.
 */

/** The top-level component or actor a design identity lies in: its first segment. */
export function scopeOf(id: string): string {
  return id.split(/[.#]/)[0] ?? id;
}

function escaped(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\-]/g, '\\$&');
}

/** A text with the references into `scope` written relative to it. */
export function relative(text: string, scope: string): string {
  return text.replace(new RegExp(`(?<![\\w.-])${escaped(scope)}\\.`, 'g'), '');
}

/** A part of a written type or signature: plain text, or the reference to the design node it names. */
export interface TypePart {
  text: string;
  id?: string;
}

/** Whether a character can stand inside an identity, so a reference found in a text is not part of a longer name. */
function inName(character: string | undefined): boolean {
  return character !== undefined && (character.toLowerCase() !== character.toUpperCase() || '0123456789_.-:/#'.includes(character));
}

/**
 * A written type or signature as parts: each of the identities `refs` gives (a node's or a member's `refers`, from the
 * interface) where the text writes it is a part of its own, written relative to `scope` when one is given; every other
 * piece of text stays plain.
 */
export function typeParts(text: string, scope: string | undefined, refs: readonly string[]): TypePart[] {
  const written = (part: string) => (scope === undefined ? part : relative(part, scope));
  // The longest identity first, so `order.OrderLine` is not taken for `order.Order`.
  const byLength = [...refs].sort((a, b) => b.length - a.length);
  const parts: TypePart[] = [];
  let plainFrom = 0;
  let at = 0;
  while (at < text.length) {
    const id = inName(text[at - 1]) ? undefined : byLength.find(ref => text.startsWith(ref, at) && !inName(text[at + ref.length]));
    if (id === undefined) {
      at += 1;
      continue;
    }
    if (at > plainFrom) {
      parts.push({ text: written(text.slice(plainFrom, at)) });
    }
    parts.push({ text: written(id), id });
    at += id.length;
    plainFrom = at;
  }
  if (text.length > plainFrom) {
    parts.push({ text: written(text.slice(plainFrom)) });
  }
  return parts;
}

/** An operation's signature, `(params) -> result raises errors`, written relative to its component. */
export function signatureOf(op: Pick<Node, 'id' | 'attrs'>): string {
  return relative(op.attrs.signature?.[0] ?? '()', scopeOf(op.id));
}

/** An operation as a card's row writes it: its name and signature. */
export function opLine(op: Pick<Node, 'id' | 'title' | 'attrs'>): string {
  return `${op.title}${signatureOf(op)}`;
}

/** The signature's call and what it raises, as parts whose references (the operation's `refers`) are apart, relative to the operation's component. */
export function signatureTypes(op: Pick<Node, 'id' | 'attrs' | 'refers'>): { call: TypePart[]; raises?: TypePart[] } {
  const scope = scopeOf(op.id);
  const [call, raises] = (op.attrs.signature?.[0] ?? '()').split(' raises ');
  return { call: typeParts(call ?? '', scope, op.refers), raises: raises === undefined ? undefined : typeParts(raises, scope, op.refers) };
}

/** A field written `name: type`, split into its name and its type. */
export function fieldParts(field: string): { name: string; type: string } {
  const at = field.indexOf(':');
  return at < 0 ? { name: field, type: '' } : { name: field.slice(0, at).trim(), type: field.slice(at + 1).trim() };
}

/** The step's index, as `#n`. */
export function stepIndex(step: Pick<Node, 'title'>): string {
  return step.title;
}

/** A step's flow-relative name, such as `submit-order#4` for `order.submit-order#4`. */
export function stepName(step: Pick<Node, 'id'>): string {
  const [flow, index] = step.id.split('#');
  return `${(flow ?? '').split('.').at(-1) ?? ''}#${index ?? ''}`;
}

function argsOf(step: Pick<Node, 'attrs'>): string {
  return (step.attrs.args ?? []).join(', ');
}

/** What a step does, as its line in the design file reads, relative to its flow's component. */
export function stepLine(step: Pick<Node, 'id' | 'form' | 'attrs'> & { step?: StepInfo }): string {
  const scope = scopeOf(step.id);
  const target = step.step?.target ? relative(step.step.target.id, scope) : '';
  const sender = step.step?.sender ? relative(step.step.sender.id, scope) : '';
  switch (step.form) {
    case 'emit':
      return `${sender} emits ${target}(${argsOf(step)})`;
    case 'on':
      return `on ${target}`;
    case 'async':
      return `async ${target}`;
    default:
      return `${sender} -> ${target}(${argsOf(step)})`;
  }
}

/** What a step calls or emits, as a message on a lifeline or a box of an activity reads: `reserve(items)`. */
export function messageOf(step: Pick<Node, 'form' | 'attrs'> & { step?: StepInfo }): string {
  return `${step.step?.target?.title ?? ''}(${argsOf(step)})`;
}

/** The code file a code element is declared in, among its ancestors; none for a design node or a file itself. */
export function fileOf(entity: Pick<Entity, 'ancestors'>): Node | undefined {
  return entity.ancestors.findLast(ancestor => ancestor.layer === 'code' && ancestor.kind === 'container' && ancestor.form === 'file');
}

/** The last segment of a path, or of a code identity's dotted name. */
export function baseName(path: string): string {
  return path.split('/').at(-1) ?? path;
}

/** A code position as a page cites it: the file's name and the line, `service.py:40`. */
export function placeOf(origin: { path: string; line: number }): string {
  return `${baseName(origin.path)}:${origin.line}`;
}

/** A path within its component's directory, as `order/service.py` for `src/shop/order/service.py`. */
export function shortPath(path: string): string {
  const parts = path.split('/');
  return parts.length > 2 ? parts.slice(-2).join('/') : path;
}

/** The status a node shows: its own, or for a code element the status of what it realizes. */
export function statusOf(node: Pick<Node, 'status' | 'bindings'>): Node['status'] {
  return node.status ?? node.bindings?.[0]?.node.status;
}

/**
 * The name a page gives a node: an operation of a type with its type's name (`OrderService.submit`), and within
 * `scope` a design node relative to it (`inventory.OutOfStock` seen from order); otherwise its title.
 */
export function displayName(node: Pick<Node, 'id' | 'title' | 'kind' | 'layer'>, scope?: string): string {
  if (node.layer !== 'design' || node.kind === 'step' || node.kind === 'container' || node.kind === 'actor') {
    return node.title;
  }
  const relativeId = relative(node.id, scopeOf(node.id));
  if (scope === undefined) {
    return node.kind === 'op' ? relativeId : node.title;
  }
  return relative(node.id, scope);
}

/** Review metadata stays in the existing review hint. */
export function reviewDescription(review: Review): string {
  return [review.decision && REVIEW_DECISIONS[review.decision], review.reviewer, review.at, review.comment, review.digest].filter(Boolean).join(' · ');
}
