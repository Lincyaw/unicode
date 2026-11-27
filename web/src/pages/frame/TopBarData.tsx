import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useSearch, useSnapshot } from '../../api/client';
import { SearchBox } from '../../components/shell/SearchBox';
import { TopBar } from '../../components/shell/TopBar';
import { entityPath } from '../../routes/address';

/** The top bar with the system's name, which enters the system's page, and the search, whose choice enters the node's page. */
export function TopBarData({ base }: { base?: string }) {
  const snapshot = useSnapshot();
  const navigate = useNavigate();
  const [text, setText] = useState('');
  const results = useSearch(text).data;
  const enter = (id: string) => entityPath({ id, trail: [{ node: id }], base });
  return (
    <TopBar
      system={snapshot.system.title}
      systemHref={enter(snapshot.system.id)}
      search={<SearchBox text={text} onText={setText} results={results} onPick={id => void navigate(enter(id))} />}
    />
  );
}
