import * as Popover from '@radix-ui/react-popover';
import { clsx } from 'clsx';
import { useState } from 'react';
import type { Node } from '../../api/types';
import { APP } from '../../terms';
import { MarkButton, SearchField } from '../../ui/controls';
import hintStyles from '../../ui/hint.module.css';
import { KindIcon } from '../marks/icons';
import { StatusDot } from '../marks/StatusDot';
import styles from './shell.module.css';

/** The key that, with the platform's modifier, puts the cursor in the search box from anywhere on the page. */
const FOCUS_KEY = 'k';

/**
 * The search box: the nodes whose identity, name or summary holds the text, listed under the box as it is typed.
 * Choosing one, by a click or Enter on the first, enters its page.
 */
export function SearchBox({ text, onText, results, onPick }: { text: string; onText: (text: string) => void; results?: Node[]; onPick: (id: string) => void }) {
  const [focused, setFocused] = useState(false);
  const pick = (id: string) => {
    onText('');
    onPick(id);
  };
  return (
    <Popover.Root open={focused && text.trim() !== ''}>
      <Popover.Anchor className={styles.search}>
        <SearchField
          label={APP.search} value={text} onChange={onText} shortcut={FOCUS_KEY} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)}
          onKeyDown={event => {
            const first = results?.[0];
            if (event.key === 'Enter' && first) {
              pick(first.id);
            }
          }}
          className={styles.searchField}
        />
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
                <MarkButton className={styles.result} onClick={() => pick(node.id)}>
                  <StatusDot status={node.status} />
                  <KindIcon node={node} size="small" />
                  <span className={clsx(styles.resultTitle, 't-code')}>{node.title}</span>
                  <span className={clsx(styles.resultId, 't-mono c-faint')}>{node.id}</span>
                </MarkButton>
              </li>
            ))}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
