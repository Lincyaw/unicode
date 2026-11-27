import { Moon } from 'lucide-react';
import type { ReactNode } from 'react';
import { APP } from '../../terms';
import { ICON_SIZE, ICON_STROKE } from '../../ui/hint';
import { toggleTheme } from '../../ui/theme';
import { Badge } from '../marks/Badge';
import styles from './shell.module.css';

/** The top bar: the wordmark, the system's name, the search box and the theme switch. */
export function TopBar({ system, search }: { system: string; search: ReactNode }) {
  return (
    <header className={styles.topBar}>
      <span className={styles.wordmark}>∞ unicode</span>
      <span className="t-small c-muted">{APP.name}</span>
      <Badge tone="neutral">{system}</Badge>
      {search}
      <button type="button" className={styles.themeSwitch} aria-label={APP.theme} onClick={toggleTheme}>
        <Moon size={ICON_SIZE} strokeWidth={ICON_STROKE} aria-hidden="true" />
      </button>
    </header>
  );
}
