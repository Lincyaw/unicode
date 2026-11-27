import { ChevronDown, ChevronRight } from 'lucide-react';
import { useState, type CSSProperties } from 'react';
import type { TreeNode } from '../../api/types';
import { ABOUT, CONTROLS } from '../../terms';
import { ICON_SMALL, ICON_STROKE, ICON_TINY, InfoHint } from '../../ui/hint';
import { KindIcon } from '../marks/icons';
import { StatusDot } from '../marks/StatusDot';
import styles from './shell.module.css';

/**
 * The design tree: the design nodes by containment. The branches toward the current node start open, and the reader
 * opens and closes the others; one click previews a node, a double click enters its page.
 */
export function DesignTree({ root, current, reveal, onSelect, onOpen }: {
  root: TreeNode;
  current?: string;
  reveal: string[];
  onSelect: (id: string) => void;
  onOpen: (id: string) => void;
}) {
  const [toggled, setToggled] = useState<ReadonlyMap<string, boolean>>(new Map());
  const opened = new Set([root.node.id, ...reveal, ...(current === undefined ? [] : [current])]);
  const isOpen = (id: string) => toggled.get(id) ?? opened.has(id);
  const toggle = (id: string) => setToggled(new Map(toggled).set(id, !isOpen(id)));

  const row = (item: TreeNode, depth: number) => {
    const { node } = item;
    const open = isOpen(node.id);
    const Chevron = open ? ChevronDown : ChevronRight;
    return (
      <li key={node.id}>
        <div className={styles.treeRow} style={{ '--depth': depth } as CSSProperties} aria-current={node.id === current ? 'true' : undefined} data-dim={node.attrs.focus?.[0] === 'utility'}>
          {item.children.length > 0
            ? (
              <button type="button" className={styles.treeToggle} aria-expanded={open} aria-label={open ? CONTROLS.collapseRow : CONTROLS.expandRow} onClick={() => toggle(node.id)}>
                <Chevron size={ICON_TINY} strokeWidth={ICON_STROKE} aria-hidden="true" />
              </button>
            )
            : <span className={styles.treeToggle} />}
          <button type="button" className={styles.treeItem} onClick={() => onSelect(node.id)} onDoubleClick={() => onOpen(node.id)}>
            <KindIcon node={node} size={ICON_SMALL} />
            <span className={styles.treeLabel}>{node.title}</span>
            <StatusDot status={node.status} />
          </button>
        </div>
        {open && item.children.length > 0 && <ul>{item.children.map(child => row(child, depth + 1))}</ul>}
      </li>
    );
  };

  return (
    <>
      <div className={styles.paneHead}>
        <h2 className="t-label">{CONTROLS.tree}</h2>
        <InfoHint label={CONTROLS.tree}>{ABOUT.tree}</InfoHint>
      </div>
      <ul className={styles.treeList}>{row(root, 0)}</ul>
    </>
  );
}
