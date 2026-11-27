import {
  Box, Braces, Cloud, Code, CornerDownRight, Eye, FileCode, FlaskConical, List, Network, OctagonX, Package, Puzzle, Radio, ShieldCheck, Split,
  SquareFunction, User, Workflow, Zap, type LucideIcon,
} from 'lucide-react';
import type { Node } from '../../api/types';
import { Icon, type IconSize } from '../../ui/icons';

/*
 * The viewer's icons, after docs/design.md §7.5's icon table: every Lucide glyph a page draws is named here, so the
 * table and the pages keep one icon for each meaning. Node kinds are drawn by KindIcon; the glyphs of actions,
 * statuses and links are passed on for the components to draw with the kit's Icon.
 */

export {
  ArrowRight, ArrowUpRight, Check, ChevronDown, ChevronRight, ChevronUp, Code, CornerDownRight, Dices, Ellipsis, EyeOff, FileCode, FlaskConical,
  GitCommitHorizontal, GitCompareArrows, ListTree, Merge, PanelRight, Play, Plus, Repeat, Split, TriangleAlert, View, X, type LucideIcon,
} from 'lucide-react';

const TYPE_ICONS: Record<string, LucideIcon> = {
  object: Braces,
  record: Braces,
  interface: Puzzle,
  enum: List,
  error: OctagonX,
  event: Zap,
};

const STEP_ICONS: Record<string, LucideIcon> = {
  on: Split,
  async: Radio,
  emit: Zap,
};

/** The icon of a node, by its kind and form; a detail function is code, not a salient function. */
function iconOf(node: Pick<Node, 'kind' | 'form' | 'layer' | 'salient'>): LucideIcon {
  switch (node.kind) {
    case 'container':
      if (node.form === 'system') {
        return Network;
      }
      if (node.form === 'file') {
        return FileCode;
      }
      return node.form === 'package' ? Package : Box;
    case 'actor':
      return node.form === 'person' ? User : Cloud;
    case 'type':
      return TYPE_ICONS[node.form] ?? Braces;
    case 'op':
      return node.layer === 'code' && !node.salient ? Code : SquareFunction;
    case 'flow':
      return Workflow;
    case 'step':
      return STEP_ICONS[node.form] ?? CornerDownRight;
    case 'property':
      return ShieldCheck;
    case 'observation':
      return Eye;
    case 'case':
      return FlaskConical;
    case 'run':
    case 'result':
    case 'review':
      return Box;
  }
}

/** A node's kind and form as its icon, at the size of the text beside it. */
export function KindIcon({ node, size = 'regular' }: { node: Pick<Node, 'kind' | 'form' | 'layer' | 'salient'>; size?: IconSize }) {
  return <Icon glyph={iconOf(node)} size={size} />;
}
