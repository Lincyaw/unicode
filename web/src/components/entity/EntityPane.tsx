import { GitCompareArrows } from '../marks/icons';
import { Link } from 'react-router';
import type { Entity, Node, ViewKind } from '../../api/types';
import { ABOUT, CHANGE_WORDS, REVIEW, SECTIONS, VIEW_NAMES } from '../../terms';

import { Section, ViewLinks } from '../details/sections';
import { Badge } from '../marks/Badge';
import { changeWords } from './changeWords';
import { KindIcon } from '../marks/icons';
import styles from './entity.module.css';
import { Icon } from '../../ui/icons';

/** An entry of the navigation trail as the pane draws it: the view or the node, with where it leads. */
export type TrailItem = { view: ViewKind; href: string } | { node: Node; href: string };

/** The entity page's right pane: the trail that led here, the related views, the change since the base and the review. */
export function EntityPane({ entity, trail, base }: { entity: Entity; trail: TrailItem[]; base?: string }) {
  const words = changeWords(entity);
  return (
    <div className={styles.pane}>
      <Section title={SECTIONS.trail} about={ABOUT.trail}>
        {trail.map((item, index) => {
          const last = index === trail.length - 1;
          const icon = 'view' in item ? <KindIcon node={{ kind: 'container', form: 'system', layer: 'design', salient: true }} size="small" /> : <KindIcon node={item.node} size="small" />;
          const label = 'view' in item ? VIEW_NAMES[item.view] : item.node.title;
          return (
            <div key={`${index} ${label}`} className={styles.trailRow} aria-current={last ? 'page' : undefined}>
              {icon}
              {last ? <span>{label}</span> : <Link to={item.href}>{label}</Link>}
            </div>
          );
        })}
      </Section>
      <ViewLinks views={entity.views} self={entity.node.id} />
      {base !== undefined && (
        <Section title={SECTIONS.changes}>
          <div className={styles.changeRow}>
            <Icon glyph={GitCompareArrows} size="small" />
            <span className="t-mono">{base}</span>
            <span className={styles.changeWords}>{words.length > 0 ? words.join(CHANGE_WORDS.separator) : CHANGE_WORDS.unchanged}</span>
          </div>
        </Section>
      )}
      {entity.review && (
        <Section title={SECTIONS.review}>
          <Badge tone="neutral">{entity.review.state === 'reviewed' ? REVIEW.reviewed : REVIEW.pending}</Badge>
        </Section>
      )}
    </div>
  );
}
