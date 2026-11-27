import { useState, type ReactNode } from 'react';
import { Frame, type Drawer } from '../../components/shell/Frame';
import { NARROW_QUERY, tokenPixels, useMediaQuery, useWindowWidth } from '../../ui/media';
import { useStoredSize } from '../../ui/storedSize';

/** Where the browser keeps the widths the reader gives the design tree and the right pane. */
const TREE_WIDTH_KEY = 'unicode-viewer.treeWidth';
const PANE_WIDTH_KEY = 'unicode-viewer.paneWidth';

/** Neither side column narrows below its minimum, and the canvas keeps at least this much between them. */
const TREE_MIN = 200;
const PANE_MIN = 240;
const CANVAS_MIN = 280;

/**
 * The frame with the widths the reader gave its side columns, held within the window and starting at base.css's
 * --tree-width and --pane-width, and the drawer the reader opened on a narrow window. `place` is the page's address:
 * going elsewhere closes the drawer, and so does widening the window past the narrow breakpoint.
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
  const narrow = useMediaQuery(NARROW_QUERY);
  const treeStart = tokenPixels('--tree-width');
  const paneStart = tokenPixels('--pane-width');
  const treeMax = Math.max(TREE_MIN, room - PANE_MIN - CANVAS_MIN);
  const [treeWidth, setTreeWidth] = useStoredSize(TREE_WIDTH_KEY, treeStart, TREE_MIN, treeMax);
  const paneMax = Math.max(PANE_MIN, room - treeWidth - CANVAS_MIN);
  const [paneWidth, setPaneWidth] = useStoredSize(PANE_WIDTH_KEY, paneStart, PANE_MIN, paneMax);
  const [opened, setOpened] = useState<{ drawer: Drawer; at: string }>();
  return (
    <Frame
      {...parts}
      treeSize={{ width: treeWidth, min: TREE_MIN, max: treeMax, onResize: setTreeWidth, onReset: () => setTreeWidth(treeStart) }}
      paneSize={{ width: paneWidth, min: PANE_MIN, max: paneMax, onResize: setPaneWidth, onReset: () => setPaneWidth(paneStart) }}
      drawer={narrow && opened?.at === place ? opened.drawer : undefined}
      onDrawer={drawer => setOpened(drawer && { drawer, at: place })}
    />
  );
}
