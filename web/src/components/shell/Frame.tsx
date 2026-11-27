import { useEffect, useId, useRef, type CSSProperties, type ReactNode } from 'react';
import { CONTROLS } from '../../terms';
import { IconButton } from '../../ui/controls';
import { Icon } from '../../ui/icons';
import { ResizeHandle } from '../../ui/resizer';
import { ListTree, PanelRight } from '../marks/icons';
import styles from './shell.module.css';

/** A side column the reader sizes: its width and bounds, and what a drag or a double click on its handle does. */
export interface ColumnSize {
  width: number;
  min: number;
  max: number;
  onResize: (width: number) => void;
  onReset: () => void;
}

/** The side column a narrow window opens over the canvas. */
export type Drawer = 'tree' | 'pane';

/**
 * The viewer's frame: the top bar, the header, then the design tree, the canvas and the right pane side by side, and
 * the status bar. It fills the window; each of the three columns scrolls on its own, and the reader sizes the two
 * side columns. Below the narrow breakpoint the canvas takes the whole width and the side columns open over it as
 * drawers, from the buttons either side of the header. An open drawer takes the focus and leaves the canvas behind it
 * inert; Escape closes it and returns the focus to its button.
 */
export function Frame({ topBar, header, tree, main, pane, statusBar, treeSize, paneSize, drawer, onDrawer }: {
  topBar: ReactNode;
  header: ReactNode;
  tree: ReactNode;
  main: ReactNode;
  pane: ReactNode;
  statusBar: ReactNode;
  treeSize: ColumnSize;
  paneSize: ColumnSize;
  drawer?: Drawer;
  onDrawer: (drawer: Drawer | undefined) => void;
}) {
  const treeId = useId();
  const paneId = useId();
  const treeColumn = useRef<HTMLElement>(null);
  const paneColumn = useRef<HTMLElement>(null);
  const treeButton = useRef<HTMLButtonElement>(null);
  const paneButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (drawer !== undefined) {
      (drawer === 'tree' ? treeColumn : paneColumn).current?.focus();
    }
  }, [drawer]);
  useEffect(() => {
    if (drawer === undefined) {
      return undefined;
    }
    // A popover or hint inside the drawer takes its Escape first, and marks it handled.
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented) {
        onDrawer(undefined);
        (drawer === 'tree' ? treeButton : paneButton).current?.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [drawer, onDrawer]);
  const toggle = (side: Drawer) => onDrawer(drawer === side ? undefined : side);
  return (
    <div className={styles.frame}>
      {topBar}
      <div className={styles.headerRow}>
        <span className={styles.drawerToggle}>
          <IconButton ref={treeButton} label={CONTROLS.tree} variant="quiet" expanded={drawer === 'tree'} controls={treeId} onClick={() => toggle('tree')}>
            <Icon glyph={ListTree} data-upright />
          </IconButton>
        </span>
        {header}
        <span className={styles.drawerToggle}>
          <IconButton ref={paneButton} label={CONTROLS.details} variant="quiet" expanded={drawer === 'pane'} controls={paneId} onClick={() => toggle('pane')}>
            <Icon glyph={PanelRight} data-upright />
          </IconButton>
        </span>
      </div>
      <div className={styles.body} data-drawer={drawer} style={{ '--tree-width': `${treeSize.width}px`, '--pane-width': `${paneSize.width}px` } as CSSProperties}>
        <nav ref={treeColumn} id={treeId} className={styles.tree} aria-label={CONTROLS.tree} tabIndex={-1}>{tree}</nav>
        <ResizeHandle
          label={CONTROLS.resizeTree} size={treeSize.width} min={treeSize.min} max={treeSize.max} grows="end" onResize={treeSize.onResize} onReset={treeSize.onReset}
          className={styles.handle}
        />
        <main className={styles.main} inert={drawer !== undefined}>{main}</main>
        <ResizeHandle
          label={CONTROLS.resizePane} size={paneSize.width} min={paneSize.min} max={paneSize.max} grows="start" onResize={paneSize.onResize} onReset={paneSize.onReset}
          className={styles.handle}
        />
        <aside ref={paneColumn} id={paneId} className={styles.pane} aria-label={CONTROLS.details} tabIndex={-1}>{pane}</aside>
      </div>
      {statusBar}
    </div>
  );
}
