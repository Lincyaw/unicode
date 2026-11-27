import { useEdge, useEntity, useSnapshot } from '../api/client';
import type { EdgeKind } from '../api/types';
import { EdgePreview } from '../components/details/EdgePreview';
import { EntityPreview } from '../components/details/EntityPreview';
import { isEdgeKey, type ViewAddress } from '../routes/address';

function NodePreview({ id, address }: { id: string; address: ViewAddress }) {
  const entity = useEntity(id, address.base);
  return <EntityPreview entity={entity} current={{ kind: address.kind, input: address.input }} />;
}

function EdgeData({ select }: { select: string }) {
  const [kind = '', from = '', to = ''] = select.split(' ');
  const edge = useEdge(kind as EdgeKind, from, to);
  return <EdgePreview edge={edge} />;
}

/** The right pane of a view: the selected node or edge, else the view's input. */
export function Preview({ address }: { address: ViewAddress }) {
  const { system } = useSnapshot();
  if (address.select !== undefined && isEdgeKey(address.select)) {
    return <EdgeData select={address.select} />;
  }
  return <NodePreview id={address.select ?? address.input ?? system.id} address={address} />;
}
