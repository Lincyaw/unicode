import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useSearch, useSnapshot } from '../../api/client';
import { SearchBox } from '../../components/shell/SearchBox';
import { TopBar } from '../../components/shell/TopBar';
import { ROOT_PATH } from '../../routes/address';
import { useTheme } from '../../ui/theme';

/**
 * The top bar with the system's name, the search and the theme switch. The system's name and a search's choice enter
 * the node's page from where the reader is: `enter` gives that page's address, its trail starting at the current view
 * or growing from the current page.
 */
export function TopBarData({ enter }: { enter: (id: string) => string }) {
  const snapshot = useSnapshot();
  const navigate = useNavigate();
  const [text, setText] = useState('');
  const [theme, toggleTheme] = useTheme();
  const results = useSearch(text).data;
  return (
    <TopBar
      system={snapshot.system.title} homeHref={ROOT_PATH} systemHref={enter(snapshot.system.id)} theme={theme} onTheme={toggleTheme}
      search={<SearchBox text={text} onText={setText} results={results} onPick={id => void navigate(enter(id))} />}
    />
  );
}
