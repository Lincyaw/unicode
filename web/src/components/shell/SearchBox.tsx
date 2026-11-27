import * as Popover from '@radix-ui/react-popover';
import { clsx } from 'clsx';
import { Search } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { Node } from '../../api/types';
import { APP } from '../../terms';
import { ICON_SMALL, ICON_STROKE } from '../../ui/hint';
import hintStyles from '../../ui/hint.module.css';
import { KindIcon } from '../marks/icons';
import { StatusDot } from '../marks/StatusDot';
import styles from './shell.module.css';

/** The key that, with ⌘ or Ctrl, puts the cursor in the search box from anywhere on the page. */
const FOCUS_KEY = 'k';

/**
 * The search box: the nodes whose identity, name or summary holds the text, listed under the box as it is typed.
 * Choosing one, by a click or Enter on the first, enters its page.
 */
export function SearchBox({ text, onText, results, onPick }: { text: string; onText: (text: string) => void; results?: Node[]; onPick: (id: string) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === FOCUS_KEY) {
        event.preventDefault();
        input.current?.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);
  const pick = (id: string) => {
    onText('');
    input.current?.blur();
    onPick(id);
  };
  return (
    <Popover.Root open={focused && text.trim() !== ''}>
      <Popover.Anchor asChild>
        <label className={styles.search}>
          <Search size={ICON_SMALL} strokeWidth={ICON_STROKE} aria-hidden="true" />
          <input
            ref={input} type="search" placeholder={APP.search} aria-label={APP.search} value={text}
            onChange={event => onText(event.target.value)} onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            onKeyDown={event => {
              const first = results?.[0];
              if (event.key === 'Enter' && first) {
                pick(first.id);
              }
            }}
          />
          <kbd className="t-mono c-faint">⌘K</kbd>
        </label>
      </Popover.Anchor>
      <Popover.Portal>
        <Popover.Content
          className={clsx(hintStyles.popover, styles.results)} align="start" sideOffset={6}
          onOpenAutoFocus={event => event.preventDefault()} onMouseDown={event => event.preventDefault()}
        >
          {results?.length === 0 && <p className="t-small c-faint">{APP.noResults}</p>}
          <ul>
            {results?.map(node => (
              <li key={node.id}>
                <button type="button" className={styles.result} onClick={() => pick(node.id)}>
                  <StatusDot status={node.status} />
                  <KindIcon node={node} size={ICON_SMALL} />
                  <span className="t-code">{node.title}</span>
                  <span className={clsx(styles.resultId, 't-mono c-faint')}>{node.id}</span>
                </button>
              </li>
            ))}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
