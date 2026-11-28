import type { ViewEdge } from '../../api/types';
import { Tag } from '../../ui/content';
import { Icon } from '../../ui/icons';
import { ViaTag } from '../details/sections';
import { Merge } from '../marks/icons';
import { markOf } from './graphParts';
import styles from './views.module.css';

/**
 * What a call carries on its line: how many detail functions a reaches edge passes, and how many calls a merged edge
 * stands for. It is drawn at the size markOf gives it, the room the layouts reserve for it, so the two change together.
 */
export function CallMark({ edge }: { edge: ViewEdge }) {
  const size = markOf(edge);
  return size && (
    <span className={styles.callMark} style={size}>
      {edge.kind === 'reaches' && <ViaTag count={edge.attrs.via?.length ?? 0} />}
      {edge.count !== undefined && <Tag><Icon glyph={Merge} size="tiny" />{edge.count}</Tag>}
    </span>
  );
}
