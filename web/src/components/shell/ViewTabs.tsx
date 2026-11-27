import { viewKindValues } from '../../api/schema';
import type { ViewKind } from '../../api/types';
import { CONTROLS, VIEW_NAMES } from '../../terms';
import { SegmentedLinks } from '../../ui/controls';

/** Where a tab leads, or why it cannot be chosen. */
export type TabLink = { href: string } | { reason: string };

/**
 * The view tabs, the entries of exploration: each is a link to its view of the current selection or a default input,
 * and a view the graph holds no input for stays faint, its reason behind a hint.
 */
export function ViewTabs({ current, tabs }: { current?: ViewKind; tabs: Record<ViewKind, TabLink> }) {
  const choices = viewKindValues.map(kind => {
    const tab = tabs[kind];
    return 'href' in tab ? { value: kind, label: VIEW_NAMES[kind], to: tab.href } : { value: kind, label: VIEW_NAMES[kind], reason: tab.reason };
  });
  return <SegmentedLinks label={CONTROLS.viewSwitch} choices={choices} value={current} />;
}
