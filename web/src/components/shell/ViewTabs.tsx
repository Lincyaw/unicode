import { viewKindValues } from '../../api/schema';
import type { ViewKind } from '../../api/types';
import { CONTROLS, VIEW_NAMES } from '../../terms';
import { SwitchLinks } from './SwitchLinks';

/**
 * The view tabs, the entries of exploration: each is a link to its view of the current selection, and a view that
 * nothing selected can take stays faint.
 */
export function ViewTabs({ current, hrefs }: { current?: ViewKind; hrefs: Partial<Record<ViewKind, string>> }) {
  return <SwitchLinks label={CONTROLS.viewSwitch} current={current} items={viewKindValues.map(kind => ({ key: kind, label: VIEW_NAMES[kind], href: hrefs[kind] }))} />;
}
