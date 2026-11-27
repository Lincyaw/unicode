import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { APP } from '../../terms';
import { ButtonLink, IconButton, ThemeIcon } from '../../ui/controls';
import type { Theme } from '../../ui/theme';
import { BrandMark } from './BrandMark';
import styles from './shell.module.css';

/** The name the wordmark spells beside its mark. */
const WORDMARK = 'unicode';

/**
 * The top bar: the wordmark, which leads home, the system's name, which leads to the system's page, the search box and
 * the switch to the other theme, which shows the theme it switches to.
 */
export function TopBar({ system, homeHref, systemHref, search, theme, onTheme }: {
  system: string; homeHref: string; systemHref: string; search: ReactNode; theme: Theme; onTheme: () => void;
}) {
  return (
    <header className={styles.topBar}>
      <Link className={styles.brand} to={homeHref}><BrandMark />{WORDMARK}</Link>
      <span className={clsx(styles.appName, 't-small c-muted')}>{APP.name}</span>
      <ButtonLink to={systemHref} variant="quiet" size="small">{system}</ButtonLink>
      {search}
      <span className={styles.topEnd}>
        <IconButton label={APP.theme} variant="quiet" onClick={onTheme}><ThemeIcon theme={theme} /></IconButton>
      </span>
    </header>
  );
}
