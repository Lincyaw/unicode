import type { ViewEdge } from '../../api/types';
import { Tag } from '../../ui/content';
import { Icon } from '../../ui/icons';
import { ViaTag } from '../details/sections';
import { Merge } from '../marks/icons';

/** What a call carries on its line: how many detail functions a reaches edge passes, and how many calls a merged edge stands for. */
export function CallMark({ edge }: { edge: ViewEdge }) {
  return (
    <>
      {edge.kind === 'reaches' && <ViaTag count={edge.attrs.via?.length ?? 0} />}
      {edge.count !== undefined && <Tag><Icon glyph={Merge} size="tiny" />{edge.count}</Tag>}
    </>
  );
}
