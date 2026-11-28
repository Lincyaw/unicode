import { useState, type ReactNode } from 'react';
import { Frame, type Drawer } from '../../components/shell/Frame';
import { tokenPixels, useWindowWidth } from '../../ui/media';
import { useStoredSize } from '../../ui/storedSize';

/** Where the browser keeps the widths the reader gives the design tree and the right pane. */
const TREE_WIDTH_KEY = 'unicode-viewer.treeWidth';
const PANE_WIDTH_KEY = 'unicode-viewer.paneWidth';

/** Neither side column narrows below its minimum. */
const TREE_MIN = 200;
const PANE_MIN = 240;

/** The canvas never narrows below this (docs/design.md §7.5). */
const CANVAS_MIN = 480;

/**
 * The side columns that lie over the canvas as drawers, given the room the canvas leaves them: the pane once the room
 * cannot hold both columns at their widths, then the tree too once it cannot hold the tree.
 */
function drawersFor(room: number, tree: number, pane: number): Drawer[] {
  if (room >= tree + pane) {
    return [];
  }
  return room >= tree ? ['pane'] : ['tree', 'pane'];
}

/**
 * The frame with the widths the reader gave its side columns, starting at base.css's --tree-width and --pane-width;
 * the side columns that the window cannot hold beside the canvas at those widths, as drawers; and the drawer the reader
 * opened. A column keeps its width whatever the window: it becomes a drawer instead of narrowing. `place` is the
 * page's address: going elsewhere closes the drawer, and so does widening the window until its column fits.
 */
export function SizedFrame({ place, ...parts }: {
  place: string;
  topBar: ReactNode;
  header: ReactNode;
  tree: ReactNode;
  main: ReactNode;
  pane: ReactNode;
  statusBar: ReactNode;
}) {
  const room = useWindowWidth();
  const treeStart = tokenPixels('--tree-width');
  const paneStart = tokenPixels('--pane-width');
  // The canvas sits between the main column's hairline borders.
  const canvas = CANVAS_MIN + 2 * tokenPixels('--size-hairline');
  const [treeWidth, setTreeWidth] = useStoredSize(TREE_WIDTH_KEY, treeStart, TREE_MIN, Number.POSITIVE_INFINITY);
  const [paneWidth, setPaneWidth] = useStoredSize(PANE_WIDTH_KEY, paneStart, PANE_MIN, Number.POSITIVE_INFINITY);
  const drawers = drawersFor(room - canvas, treeWidth, paneWidth);
  // A drag widens a column in the row only as far as the canvas keeps its minimum.
  const treeMax = Math.max(TREE_MIN, room - canvas - (drawers.includes('pane') ? 0 : paneWidth));
  const paneMax = Math.max(PANE_MIN, room - canvas - treeWidth);
  const [opened, setOpened] = useState<{ drawer: Drawer; at: string }>();
  // A drawer is closed for good once its column is back in the row or the reader moves elsewhere, so narrowing the
  // window later, or coming back to the same place, does not reopen it.
  if (opened !== undefined && (!drawers.includes(opened.drawer) || opened.at !== place)) {
    setOpened(undefined);
  }
  return (
    <Frame
      {...parts}
      treeSize={{ width: treeWidth, min: TREE_MIN, max: treeMax, onResize: width => setTreeWidth(Math.min(width, treeMax)), onReset: () => setTreeWidth(treeStart) }}
      paneSize={{ width: paneWidth, min: PANE_MIN, max: paneMax, onResize: width => setPaneWidth(Math.min(width, paneMax)), onReset: () => setPaneWidth(paneStart) }}
      drawers={drawers}
      drawer={opened?.drawer}
      onDrawer={drawer => setOpened(drawer && { drawer, at: place })}
    />
  );
}
