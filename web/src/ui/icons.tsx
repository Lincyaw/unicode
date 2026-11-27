import { clsx } from 'clsx';
import type { LucideIcon } from 'lucide-react';
import styles from './icons.module.css';

/*
 * Every icon is a Lucide line icon, coloured like its text and stroked at 1.4. Its size is one of three, defined here
 * once: the page's components draw icons with Icon, and layout code that measures a row reads the sizes in pixels.
 */

/** Where an icon stands: in a row of text, beside small text, or inside a tag. */
export type IconSize = 'regular' | 'small' | 'tiny';

export const ICON_REGULAR = 16;
export const ICON_SMALL = 14;
export const ICON_TINY = 12;

const ICON_SIZES: Record<IconSize, number> = { regular: ICON_REGULAR, small: ICON_SMALL, tiny: ICON_TINY };

export const ICON_STROKE = 1.4;

/**
 * A line icon at one of the three sizes; it never shrinks in a row, and screen readers skip it. Data attributes tell a
 * control how the icon moves, such as an arrow's direction.
 */
export function Icon({ glyph: Glyph, size = 'regular', className, ...data }: { glyph: LucideIcon; size?: IconSize; className?: string } & { [key: `data-${string}`]: string | boolean }) {
  return <Glyph {...data} className={clsx(styles.icon, className)} size={ICON_SIZES[size]} strokeWidth={ICON_STROKE} aria-hidden="true" />;
}
