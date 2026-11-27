import { Navigate, useNavigate, useSearchParams } from 'react-router';
import { useEntity, useNodes } from '../api/client';
import { EntityBody } from '../components/entity/EntityBody';
import { EntityPane, type TrailItem } from '../components/entity/EntityPane';
import entityStyles from '../components/entity/entity.module.css';
import { LinksContext, type NodeLinks } from '../components/marks/links';
import { Frame } from '../components/shell/Frame';
import { PageHeader } from '../components/shell/PageHeader';
import { ViewTabs } from '../components/shell/ViewTabs';
import {
  entityPath, following, isEdgeKey, readEntityAddress, requestedView, ROOT_PATH, trailView, viewPath, type EntityAddress,
} from '../routes/address';
import { displayName } from '../components/marks/format';
import { kindWord } from '../terms';
import { Boundary } from './Boundary';
import { HeaderTools } from './frame/HeaderTools';
import { StatusBarData } from './frame/StatusBarData';
import { tabLinks } from './frame/tabs';
import { TopBarData } from './frame/TopBarData';
import { TreeData } from './frame/TreeData';

/** The links of an entity page: every node it shows leads to that node's page, the trail growing by it. */
function entityLinks(address: EntityAddress, navigate: (to: string) => void): NodeLinks {
  const href = (id: string) => entityPath(following(address, id));
  return {
    selected: address.id,
    select: id => navigate(href(id)),
    open: id => navigate(href(id)),
    entityHref: href,
    viewHref: request => viewPath(requestedView(request, address.base)),
  };
}

function EntityHeader({ address, links }: { address: EntityAddress; links: NodeLinks }) {
  const entity = useEntity(address.id, address.base);
  const tabs = tabLinks(entity.tabs, (_kind, target) => viewPath({ ...target, base: address.base }));
  const { node } = entity;
  return (
    <PageHeader crumbs={[...entity.ancestors.map(ancestor => ({ label: ancestor.title, to: links.entityHref(ancestor.id) })), { label: kindWord(node.kind, node.form, node.layer) }]} title={displayName(node)}>
      <ViewTabs tabs={tabs} />
      <HeaderTools
        base={address.base}
        baseHref={base => entityPath({ ...address, base })}
        subjectHref={subject => (isEdgeKey(subject) ? undefined : links.entityHref(subject))}
        entityHref={links.entityHref}
      />
    </PageHeader>
  );
}

function EntityMain({ address }: { address: EntityAddress }) {
  const entity = useEntity(address.id, address.base);
  return <div className={entityStyles.scroll}><EntityBody entity={entity} /></div>;
}

function EntityTrail({ address }: { address: EntityAddress }) {
  const entity = useEntity(address.id, address.base);
  const ids = address.trail.flatMap(entry => ('node' in entry ? [entry.node] : []));
  const nodes = new Map(useNodes(ids).map(node => [node.id, node]));
  const trail = address.trail.flatMap((entry, index): TrailItem[] => {
    if ('view' in entry) {
      return [{ view: entry.view, href: viewPath(trailView(entry, address.base)) }];
    }
    const node = nodes.get(entry.node);
    return node ? [{ node, href: entityPath({ id: node.id, trail: address.trail.slice(0, index + 1), base: address.base }) }] : [];
  });
  return <EntityPane entity={entity} trail={trail} base={address.base} />;
}

function EntityTree({ address, links }: { address: EntityAddress; links: NodeLinks }) {
  const entity = useEntity(address.id, address.base);
  return <TreeData current={address.id} reveal={entity.ancestors.map(node => node.id)} onSelect={links.select} onOpen={links.open} />;
}

/** A node's entity page (docs/design.md §7.5): the node in full on the canvas, the trail that led here on the right. */
export function EntityPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const address = readEntityAddress(params);
  if (address === undefined) {
    return <Navigate to={ROOT_PATH} replace />;
  }
  const links = entityLinks(address, to => void navigate(to));
  const key = entityPath(address);
  return (
    <LinksContext value={links}>
      <Frame
        topBar={<Boundary reset="top" quiet><TopBarData base={address.base} /></Boundary>}
        header={<Boundary reset={key} quiet><EntityHeader address={address} links={links} /></Boundary>}
        tree={<Boundary reset={address.id}><EntityTree address={address} links={links} /></Boundary>}
        main={<Boundary reset={key}><EntityMain address={address} /></Boundary>}
        pane={<Boundary reset={key}><EntityTrail address={address} /></Boundary>}
        statusBar={<Boundary reset="status" quiet><StatusBarData /></Boundary>}
      />
    </LinksContext>
  );
}
