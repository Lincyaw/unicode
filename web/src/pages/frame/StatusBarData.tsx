import { useSnapshot } from '../../api/client';
import { StatusBar } from '../../components/shell/StatusBar';

export function StatusBarData() {
  const { head, counts } = useSnapshot();
  return <StatusBar branch={head.branch} commit={head.short} dirty={head.dirty} diagnostics={counts.diagnostics} />;
}
