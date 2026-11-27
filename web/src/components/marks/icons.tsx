import {
  Box, Braces, Cloud, Code, CornerDownRight, Eye, FileCode, FlaskConical, List, Network, OctagonX, Package, Puzzle, Radio, ShieldCheck, Split,
  SquareFunction, User, Workflow, Zap, type LucideIcon,
} from 'lucide-react';
import { createElement } from 'react';
import type { Node } from '../../api/types';
import { ICON_SIZE, ICON_STROKE } from '../../ui/hint';

/*
 * Node kinds are icons from Lucide (docs/design.md §7.5's icon table), 16px with a 1.4 stroke, coloured like their
 * text; a smaller size is given where the text beside it is smaller.
 */

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

export function KindIcon({ node, size = ICON_SIZE }: { node: Pick<Node, 'kind' | 'form' | 'layer' | 'salient'>; size?: number }) {
  return createElement(iconOf(node), { size, strokeWidth: ICON_STROKE, 'aria-hidden': true });
}
