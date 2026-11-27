import { useNavigate, useParams, useSearchParams } from 'react-router';
import { useOptionalEntity, useSnapshot, useView } from '../api/client';
import { LinksContext, type NodeLinks } from '../components/marks/links';
import { Frame } from '../components/shell/Frame';
import { PageHeader } from '../components/shell/PageHeader';
import { ViewTabs } from '../components/shell/ViewTabs';
import { ViewDrawing } from './ViewDrawing';
import {
  DEFAULT_VIEW, enteringFromView, entityPath, isEdgeKey, readViewAddress, readViewKind, requestedView, selecting, toggling, viewPath, type ViewAddress,
} from '../routes/address';
import { displayName } from '../components/marks/format';
import { kindWord, VIEW_NAMES, VIEW_WORD } from '../terms';
import { Boundary } from './Boundary';
import { HeaderTools } from './frame/HeaderTools';
import { StatusBarData } from './frame/StatusBarData';
import { tabLinks } from './frame/tabs';
import { TopBarData } from './frame/TopBarData';
import { TreeData } from './frame/TreeData';
import { Preview } from './Preview';

/** The links of a view page: a click selects in place, a double click enters the entity page with the view as its trail's start. */
function viewLinks(address: ViewAddress, navigate: (to: string, options?: { replace: boolean }) => void): NodeLinks {
  return {
    selected: address.select,
    select: id => navigate(viewPath(selecting(address, id)), { replace: true }),
    open: id => navigate(entityPath(enteringFromView(address, id))),
    entityHref: id => entityPath(enteringFromView(address, id)),
    viewHref: request => viewPath(requestedView(request, address.base)),
    expandHref: id => viewPath(toggling(address, id)),
  };
}

/** The header of a view: the path to its input and the view's name with the input's, then the tabs and tools. */
function ViewHeader({ address, links }: { address: ViewAddress; links: NodeLinks }) {
  const view = useView(address);
  const { system } = useSnapshot();
  // The tabs lead from the selected node, once its page has arrived, else from the view's input.
  const selection = useOptionalEntity(address.select !== undefined && !isEdgeKey(address.select) ? address.select : undefined);
  const tabs = tabLinks(selection?.tabs ?? view.tabs, (kind, target) => viewPath(kind === address.kind ? address : { ...target, base: address.base }));
  const onSystem = view.input.form === 'system';
  const kind = kindWord(view.input.kind, view.input.form, view.input.layer);
  const crumbs = [...(onSystem ? [system] : view.ancestors).map(node => ({ label: node.title, to: links.entityHref(node.id) })), { label: onSystem ? VIEW_WORD : kind }];
  return (
    <PageHeader
      crumbs={crumbs}
      title={onSystem ? VIEW_NAMES[address.kind] : <>{VIEW_NAMES[address.kind]}<span aria-hidden="true">·</span>{displayName(view.input)}</>}
    >
      <ViewTabs current={address.kind} tabs={tabs} />
      <HeaderTools
        base={address.base}
        baseHref={base => viewPath({ ...address, base })}
        subjectHref={subject => (isEdgeKey(subject) ? viewPath(selecting(address, subject)) : links.entityHref(subject))}
        entityHref={links.entityHref}
      />
    </PageHeader>
  );
}

/** The tree of a view page, opened toward the selected node or the view's input. */
function ViewTree({ address, links }: { address: ViewAddress; links: NodeLinks }) {
  const current = address.select !== undefined && !isEdgeKey(address.select) ? address.select : address.input;
  const entity = useOptionalEntity(current);
  return <TreeData current={current} reveal={entity?.ancestors.map(node => node.id) ?? []} onSelect={links.select} onOpen={links.open} />;
}

/** A view (docs/design.md §7.1): the drawing, the design tree and the preview of the selection. */
export function ViewPage() {
  const kind = readViewKind(useParams().kind) ?? DEFAULT_VIEW;
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const address = readViewAddress(kind, params);
  const links = viewLinks(address, (to, options) => void navigate(to, options));
  const key = viewPath(address);
  const drawn = viewPath({ ...address, select: undefined, base: undefined });
  return (
    <LinksContext value={links}>
      <Frame
        topBar={<Boundary reset="top" quiet><TopBarData base={address.base} /></Boundary>}
        header={<Boundary reset={key} quiet><ViewHeader address={address} links={links} /></Boundary>}
        tree={<Boundary reset="tree"><ViewTree address={address} links={links} /></Boundary>}
        main={<Boundary reset={drawn}><ViewDrawing address={address} /></Boundary>}
        pane={<Boundary reset={key}><Preview address={address} /></Boundary>}
        statusBar={<Boundary reset="status" quiet><StatusBarData /></Boundary>}
      />
    </LinksContext>
  );
}
