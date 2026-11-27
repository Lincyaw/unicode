import { useTree } from '../../api/client';
import { DesignTree } from '../../components/shell/DesignTree';

/** The design tree, opened toward the current node. */
export function TreeData({ current, reveal, onSelect, onOpen }: { current?: string; reveal: string[]; onSelect: (id: string) => void; onOpen: (id: string) => void }) {
  const root = useTree();
  return <DesignTree root={root} current={current} reveal={reveal} onSelect={onSelect} onOpen={onOpen} />;
}
