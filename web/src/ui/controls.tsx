import * as RadixSelect from '@radix-ui/react-select';
import { clsx } from 'clsx';
import {
  ArrowUpRight, Check, ChevronDown, ChevronLeft, ChevronRight, Copy, File, Maximize, Menu, Minimize, Minus, Moon, Plus, Search, Sun, X,
} from 'lucide-react';
import { useEffect, useId, useLayoutEffect, useRef, useState, type ComponentProps, type KeyboardEvent, type ReactNode, type RefObject } from 'react';
import { Link } from 'react-router';
import styles from './controls.module.css';
import { Hint } from './hint';
import { Icon } from './icons';
import type { Theme } from './theme';
import { UI_WORDS } from './words';

/**
 * How a control stands in its bar: outlined with the ∞ edge, or quiet, its outline left out, so that the one outlined
 * control of the bar stands out.
 */
export type ControlVariant = 'line' | 'quiet';

/** A solid button leads the page forward and carries an arrow; a line or quiet button is any other action. */
type ButtonVariant = 'solid' | ControlVariant;

/** A pill's height: large for a page's actions, medium for a bar's controls, small for a dense row such as a tool bar or a list. */
export type PillSize = 'large' | 'medium' | 'small';

/** The classes of a pill that acts or leads somewhere, by its variant and size. */
function buttonClasses(variant: ButtonVariant, size: PillSize): string {
  return clsx(styles.pill, styles[size], variant === 'solid' && styles.solid, variant === 'quiet' ? styles.quiet : 'loop-edge');
}

export function ButtonLink({ to, variant, size = 'large', children }: { to: string; variant: ButtonVariant; size?: PillSize; children: ReactNode }) {
  return <Link className={buttonClasses(variant, size)} to={to}>{children}</Link>;
}

/**
 * A pill that runs an action. `label` names it for screen readers when its words alone do not, such as a count; a
 * popover's trigger hands it handlers and a ref as props, which the spread passes on.
 */
export function Button({ variant, size = 'large', label, onClick, children, ...trigger }: {
  variant: ButtonVariant; size?: PillSize; label?: string; onClick?: () => void; children: ReactNode;
}) {
  return <button {...trigger} type="button" className={buttonClasses(variant, size)} aria-label={label} onClick={onClick}>{children}</button>;
}

/**
 * A mark that acts when pressed, such as a node on a drawing, a row of a tree or a result in a list. It is data before
 * it is a control, so it is no pill: its class gives its look, and it keeps the kit's focus ring and transitions.
 */
export function MarkButton({ className, children, ...props }: Omit<ComponentProps<'button'>, 'type'>) {
  return <button {...props} type="button" className={clsx(styles.mark, className)}>{children}</button>;
}

/**
 * A button that submits its form, the one it sits in or the one its form id names; it waits, faint, while the
 * submission is under way.
 */
export function SubmitButton({ variant, pending, form, children }: { variant: ButtonVariant; pending: boolean; form?: string; children: ReactNode }) {
  return (
    <button type="submit" form={form} className={buttonClasses(variant, 'large')} disabled={pending} aria-busy={pending}>
      {children}
    </button>
  );
}

/**
 * A labelled pill field for a line of text; the label sits above it. A field with a pattern has the browser hold back
 * the form until the text matches, and its hint says what matches.
 */
export function TextField({ label, type, autoComplete, value, onChange, pattern }: {
  label: string; type: 'text' | 'password'; autoComplete: string; value: string; onChange: (value: string) => void; pattern?: { regex: string; hint: string };
}) {
  return (
    <label className={styles.textField}>
      <span className="t-micro c-muted">{label}</span>
      <input
        className={clsx(styles.pill, styles.large, styles.input)} type={type} autoComplete={autoComplete} required value={value}
        pattern={pattern?.regex} title={pattern?.hint} onChange={event => onChange(event.target.value)}
      />
      {pattern && <span className="t-micro c-faint">{pattern.hint}</span>}
    </label>
  );
}

/** A labelled field for a few lines of text, which a form needs filled; the label sits above it. */
export function TextArea({ label, value, onChange, rows }: { label: string; value: string; onChange: (value: string) => void; rows: number }) {
  return (
    <label className={styles.textField}>
      <span className="t-micro c-muted">{label}</span>
      <textarea className={styles.area} rows={rows} required value={value} onChange={event => onChange(event.target.value)} />
    </label>
  );
}

/** A pill that is on or off, such as one choice among many that can each be chosen; it carries a check while on. */
export function ToggleChip({ pressed, onToggle, disabled = false, children }: { pressed: boolean; onToggle: () => void; disabled?: boolean; children: ReactNode }) {
  return (
    <button type="button" className={clsx(styles.pill, styles.medium, styles.chip, 'loop-edge')} aria-pressed={pressed} disabled={disabled} onClick={onToggle}>
      <span className={styles.chipCheck}><Icon glyph={Check} size="tiny" /></span>
      {children}
    </button>
  );
}

/** Copies a text and confirms beside the button, drawing a check, once the text is on the clipboard. */
export function CopyButton({ label, text }: { label: string; text: () => string }) {
  const [copied, setCopied] = useState(false);
  return (
    <span className={styles.copy}>
      <Button variant="line" onClick={() => void navigator.clipboard.writeText(text()).then(() => setCopied(true))}>
        <Icon glyph={Copy} size="small" />
        {label}
      </Button>
      {copied && (
        <span className={clsx(styles.copied, 't-micro c-ok')} role="status">
          <Icon glyph={Check} size="tiny" />
          {UI_WORDS.copied}
        </span>
      )}
    </span>
  );
}

/** How large a control without words is: the round control of a bar, or a small one beside small pills in a dense row. */
export type IconButtonSize = 'regular' | 'small';

/**
 * A round control whose icon turns when hovered; a disabled one stays still and faint. It runs an action, opens a page
 * of the site, or opens an address elsewhere in a new tab.
 */
export function IconButton({ label, expanded, disabled, variant = 'line', size = 'regular', children, ...action }: {
  label: string; expanded?: boolean; disabled?: boolean; variant?: ControlVariant; size?: IconButtonSize; children: ReactNode;
} & ({ onClick: () => void } | { to: string } | { href: string })) {
  const quiet = variant === 'quiet';
  const className = clsx(styles.pill, styles.icon, size === 'small' && styles.small, quiet && styles.quiet, !disabled && !quiet && 'loop-edge');
  // A hint makes the control its trigger by handing it handlers and a ref as props, which the spread passes on.
  if ('to' in action) {
    return <Link {...action} className={className} aria-label={label}>{children}</Link>;
  }
  if ('href' in action) {
    return <a {...action} className={className} target="_blank" rel="noreferrer" aria-label={label}>{children}</a>;
  }
  return <button {...action} type="button" className={className} aria-label={label} aria-expanded={expanded} disabled={disabled}>{children}</button>;
}

/** Appends a list's next page; its label says how much is shown, and it waits while the page loads. */
export function MoreButton({ label, loading, onClick, className }: { label: string; loading: boolean; onClick: () => void; className?: string }) {
  return <button type="button" className={clsx(styles.more, 't-micro', className)} disabled={loading} aria-busy={loading} onClick={onClick}>{label}</button>;
}

export interface Choice<T extends string> {
  value: T;
  label: string;
}

/** A caption over a run of consecutive choices, such as the stage a few benchmarks belong to; `span` counts the choices. */
export interface ChoiceCaption {
  label: ReactNode;
  span: number;
}

/** The thumb under the chosen choice of a track, which slides when another is chosen; choices mark themselves `data-value`. */
function useThumb(track: RefObject<HTMLElement | null>, chosen: string | undefined): { left: number; width: number } | null {
  const [thumb, setThumb] = useState<{ left: number; width: number } | null>(null);
  useLayoutEffect(() => {
    const trackElement = track.current;
    if (!trackElement) {
      throw new Error('The segmented control renders its track before its layout effect runs');
    }
    const choice = chosen === undefined ? null : trackElement.querySelector<HTMLElement>(`[data-value="${CSS.escape(chosen)}"]`);
    const place = () => setThumb(choice && { left: choice.offsetLeft, width: choice.offsetWidth });
    place();
    // Choices change width when fonts or labels arrive.
    const observer = new ResizeObserver(place);
    trackElement.querySelectorAll('[data-value]').forEach(each => observer.observe(each));
    return () => observer.disconnect();
  }, [track, chosen]);
  return thumb;
}

function Thumb({ thumb }: { thumb: { left: number; width: number } | null }) {
  // The thumb mounts once it has a place, so it appears there instead of sliding in.
  return thumb && <span className={styles.thumb} style={{ width: thumb.width, transform: `translateX(${thumb.left}px)` }} aria-hidden="true" />;
}

/**
 * A row of mutually exclusive choices inside one track, which is the control and carries the ∞ edge. A thumb under the
 * chosen choice slides to the next one chosen. Captions, when given, sit over the runs of choices they name.
 */
export function SegmentedControl<T extends string>({ label, choices, value, onChange, captions }: {
  label: string; choices: readonly Choice<T>[]; value: T; onChange: (value: T) => void; captions?: readonly ChoiceCaption[];
}) {
  const track = useRef<HTMLDivElement>(null);
  const thumb = useThumb(track, value);
  const segmented = (
    <div ref={track} className={clsx(styles.segmented, 'loop-edge')} role="group" aria-label={label}>
      <Thumb thumb={thumb} />
      {choices.map(choice => (
        <button key={choice.value} type="button" value={choice.value} data-value={choice.value} className={styles.segment} aria-pressed={choice.value === value} onClick={() => onChange(choice.value)}>{choice.label}</button>
      ))}
    </div>
  );
  return captions ? (
    <div className={styles.captioned} style={{ gridTemplateColumns: `repeat(${choices.length}, auto)` }}>
      {captions.map((caption, index) => <span key={index} className={clsx(styles.caption, 't-micro c-faint')} style={{ gridColumn: `span ${caption.span}` }}>{caption.label}</span>)}
      {segmented}
    </div>
  ) : segmented;
}

/** A choice that is a place: where it leads, or, while it cannot be chosen, why not. */
export type PlaceChoice<T extends string> = Choice<T> & ({ to: string } | { reason?: string });

/**
 * A place that cannot be chosen now: a disabled link, faint, which keeps the keyboard's focus so that its reason, behind
 * a hint, reaches the eye and, as its description, assistive technology.
 */
function FaintChoice({ value, label, reason }: { value: string; label: string; reason?: string }) {
  const described = useId();
  const faint = (
    <span data-value={value} className={styles.segment} role="link" aria-disabled="true" tabIndex={0} aria-describedby={reason === undefined ? undefined : described}>{label}</span>
  );
  return reason === undefined ? faint : (
    <>
      <Hint content={reason}>{faint}</Hint>
      <span id={described} className="visually-hidden">{reason}</span>
    </>
  );
}

/**
 * The link variant of the segmented control, for choices that are places, such as the views of one subject: each choice
 * is a link, the current one sits on the thumb, and one that cannot be chosen now stays faint, its reason behind a hint.
 */
export function SegmentedLinks<T extends string>({ label, choices, value }: { label: string; choices: readonly PlaceChoice<T>[]; value?: T }) {
  const track = useRef<HTMLElement>(null);
  const thumb = useThumb(track, value);
  return (
    <nav ref={track} className={clsx(styles.segmented, 'loop-edge')} aria-label={label}>
      <Thumb thumb={thumb} />
      {choices.map(choice => {
        if ('to' in choice) {
          return <Link key={choice.value} data-value={choice.value} className={styles.segment} to={choice.to} aria-current={choice.value === value ? 'page' : undefined}>{choice.label}</Link>;
        }
        return <FaintChoice key={choice.value} value={choice.value} label={choice.label} reason={choice.reason} />;
      })}
    </nav>
  );
}

/** Whether the reader's keyboard is Apple's, whose shortcuts take the command key where others take Ctrl. */
const APPLE_KEYS = /Mac|iPhone|iPad/.test(navigator.userAgent);

/** The modifier a shortcut takes, as the reader's keyboard labels it. */
const MODIFIER_KEY = APPLE_KEYS ? '⌘' : 'Ctrl';

/**
 * A search box; `className` lets the parent place and size it. With a `shortcut` key, the platform's modifier and that
 * key put the cursor in the box from anywhere on the page, and the box shows them: ⌘K on macOS, Ctrl K elsewhere. A
 * parent that lists results under the box follows its focus and its keys.
 */
export function SearchField({ label, value, onChange, shortcut, onFocus, onBlur, onKeyDown, className }: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  shortcut?: string;
  onFocus?: () => void;
  onBlur?: () => void;
  onKeyDown?: (event: KeyboardEvent<HTMLInputElement>) => void;
  className?: string;
}) {
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (shortcut === undefined) {
      return undefined;
    }
    const onKey = (event: globalThis.KeyboardEvent) => {
      if ((APPLE_KEYS ? event.metaKey : event.ctrlKey) && event.key === shortcut) {
        event.preventDefault();
        input.current?.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [shortcut]);
  return (
    <label className={clsx(styles.pill, styles.medium, styles.field, className)}>
      <Icon glyph={Search} size="small" />
      <input
        ref={input} type="search" placeholder={label} aria-label={label} value={value} onChange={event => onChange(event.target.value)}
        onFocus={onFocus} onBlur={onBlur} onKeyDown={onKeyDown}
      />
      {shortcut !== undefined && <kbd className={styles.keys}><span>{MODIFIER_KEY}</span><span>{shortcut.toUpperCase()}</span></kbd>}
    </label>
  );
}

/** An arrow for stepping back or forth; in an icon button it nudges its way on hover instead of turning. */
export function ArrowIcon({ direction }: { direction: 'left' | 'right' }) {
  return <Icon glyph={direction === 'left' ? ChevronLeft : ChevronRight} size="tiny" data-direction={direction} />;
}

export function ChevronIcon() {
  return <Icon glyph={ChevronDown} size="tiny" />;
}

/** The theme to switch to: a sun while the page is dark, a moon while it is light. */
export function ThemeIcon({ theme }: { theme: Theme }) {
  return <Icon glyph={theme === 'dark' ? Sun : Moon} />;
}

export function MenuIcon() {
  return <Icon glyph={Menu} />;
}

/** A cross, for closing or taking something out. */
export function CloseIcon() {
  return <Icon glyph={X} size="tiny" />;
}

/** An arrow up and out, for a link to another page; in an icon button it nudges its way on hover. */
export function LinkArrowIcon() {
  return <Icon glyph={ArrowUpRight} size="tiny" data-direction="up-right" />;
}

/** A page, for a record's files; in an icon button it keeps upright on hover. */
export function FileIcon() {
  return <Icon glyph={File} size="tiny" data-upright />;
}

/** Corners pointing out, to fill the screen, or in, to leave it; in an icon button they keep upright on hover. */
export function FullscreenIcon({ leave }: { leave: boolean }) {
  return <Icon glyph={leave ? Minimize : Maximize} size="tiny" data-upright />;
}

/** A plus or a minus, for making something larger or smaller; in an icon button it keeps upright on hover. */
export function ZoomIcon({ direction }: { direction: 'in' | 'out' }) {
  return <Icon glyph={direction === 'in' ? Plus : Minus} size="tiny" data-upright />;
}

/** Four corners around the room, for fitting a drawing to the room it has; in an icon button it keeps upright on hover. */
export function FitIcon() {
  return <Icon glyph={Maximize} size="tiny" data-upright />;
}

/**
 * A pill that opens a glass list of choices; the chosen one carries a check. A quiet one keeps its chevron, so it still
 * reads as a choice to open. `className` lets the parent place it.
 */
export function Select<T extends string>({ label, choices, value, onChange, variant = 'line', className }: {
  label: string; choices: readonly Choice<T>[]; value: T; onChange: (value: T) => void; variant?: ControlVariant; className?: string;
}) {
  return (
    <RadixSelect.Root value={value} onValueChange={next => onChange(next as T)}>
      <RadixSelect.Trigger className={clsx(styles.pill, styles.medium, styles.trigger, variant === 'quiet' ? styles.quiet : 'loop-edge', className)} aria-label={label}>
        <RadixSelect.Value />
        <RadixSelect.Icon className={styles.chevron}><ChevronIcon /></RadixSelect.Icon>
      </RadixSelect.Trigger>
      <RadixSelect.Portal>
        <RadixSelect.Content className={styles.menu} position="popper" sideOffset={6} collisionPadding={12}>
          <RadixSelect.ScrollUpButton className={styles.scroll}><span className={styles.up}><ChevronIcon /></span></RadixSelect.ScrollUpButton>
          <RadixSelect.Viewport className={styles.options}>
            {choices.map(choice => (
              <RadixSelect.Item key={choice.value} value={choice.value} className={clsx(styles.option, 't-small')}>
                <RadixSelect.ItemText>{choice.label}</RadixSelect.ItemText>
                <RadixSelect.ItemIndicator className={styles.check}><Icon glyph={Check} size="tiny" /></RadixSelect.ItemIndicator>
              </RadixSelect.Item>
            ))}
          </RadixSelect.Viewport>
          <RadixSelect.ScrollDownButton className={styles.scroll}><ChevronIcon /></RadixSelect.ScrollDownButton>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    </RadixSelect.Root>
  );
}

/**
 * The scope a page keeps to, such as the model the reader came from, named in a small pill whose cross leads back to
 * everything: `to` is the page's address without the scope, and it replaces the current entry in the history.
 */
export function ScopeChip({ name, to }: { name: string; to: string }) {
  const clear = UI_WORDS.clearScope(name);
  return (
    <Hint content={clear}>
      <Link className={clsx(styles.pill, styles.small, styles.scope, 'loop-edge')} aria-label={clear} to={to} replace preventScrollReset>
        {name}<Icon glyph={X} size="tiny" />
      </Link>
    </Hint>
  );
}
