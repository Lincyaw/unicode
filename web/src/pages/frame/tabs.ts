import { viewKindValues } from '../../api/schema';
import type { ViewKind, ViewTab } from '../../api/types';
import type { TabLink } from '../../components/shell/ViewTabs';
import { requestedView, type ViewAddress } from '../../routes/address';
import { CLOSED_TABS } from '../../terms';

/**
 * The view tabs' links from the tabs the server gives (docs/design.md §7.5.1): each view's address, which the page
 * writes with the base it compares with, or why the view cannot be opened.
 */
export function tabLinks(tabs: ViewTab[], href: (kind: ViewKind, address: ViewAddress) => string): Record<ViewKind, TabLink> {
  return Object.fromEntries(viewKindValues.map((kind): [ViewKind, TabLink] => {
    const tab = tabs.find(each => each.kind === kind);
    if (tab?.view) {
      return [kind, { href: href(kind, requestedView(tab.view, undefined)) }];
    }
    if (tab?.closed === undefined) {
      throw new Error(`The answer gives the ${kind} tab neither a view nor a reason`);
    }
    return [kind, { reason: CLOSED_TABS[tab.closed] }];
  })) as Record<ViewKind, TabLink>;
}
