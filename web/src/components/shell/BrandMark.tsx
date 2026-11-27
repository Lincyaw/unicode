import styles from './shell.module.css';

/**
 * One lobe of the ∞, a lemniscate of Bernoulli: x = a·cos t / (1 + sin²t), y = a·sin t·cos t / (1 + sin²t). The design
 * half is the left lobe, the code half the right one.
 */
function lobePath(half: 'left' | 'right', cx: number, cy: number, a: number, segments: number): string {
  const from = half === 'left' ? Math.PI / 2 : -Math.PI / 2;
  const points = Array.from({ length: segments + 1 }, (_, index) => {
    const t = from + (Math.PI * index) / segments;
    const d = 1 + Math.sin(t) ** 2;
    return `${(cx + (a * Math.cos(t)) / d).toFixed(2)} ${(cy + (a * Math.sin(t) * Math.cos(t)) / d).toFixed(2)}`;
  });
  return `M ${points.join(' L ')}`;
}

/** The mark's width and height in pixels, and how finely each lobe is drawn. */
const MARK_WIDTH = 22;
const MARK_HEIGHT = 10;
const LOBE_SEGMENTS = 32;

/** The ∞ mark, its two lobes in the loop's two colours, as aegis-bench-site draws its brand. */
export function BrandMark() {
  const a = MARK_WIDTH / 2 - 1;
  return (
    <svg className={styles.brandMark} width={MARK_WIDTH} height={MARK_HEIGHT} viewBox={`0 0 ${MARK_WIDTH} ${MARK_HEIGHT}`} aria-hidden="true">
      <path d={lobePath('left', MARK_WIDTH / 2, MARK_HEIGHT / 2, a, LOBE_SEGMENTS)} />
      <path d={lobePath('right', MARK_WIDTH / 2, MARK_HEIGHT / 2, a, LOBE_SEGMENTS)} />
    </svg>
  );
}
