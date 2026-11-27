import { viewKindValues } from '../../api/schema';
import type { ViewKind } from '../../api/types';
import { CONTROLS, VIEW_NAMES } from '../../terms';
import { SwitchLinks } from './SwitchLinks';

/** Where a tab leads, or why it cannot be chosen. */
export type TabLink = { href: string } | { reason: string };

/**
 * The view tabs, the entries of exploration: each is a link to its view of the current selection or a default input,
 * and a view the graph holds no input for stays faint, its reason behind a hint.
 */
export function ViewTabs({ current, tabs }: { current?: ViewKind; tabs: Record<ViewKind, TabLink> }) {
  return <SwitchLinks label={CONTROLS.viewSwitch} current={current} items={viewKindValues.map(kind => ({ key: kind, label: VIEW_NAMES[kind], ...tabs[kind] }))} />;
}
