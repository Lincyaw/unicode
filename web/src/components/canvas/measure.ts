/*
 * How wide a text is drawn, worked out without the page: the monospaced face advances 0.6em a glyph, a Han glyph takes
 * a full em, and the text face's Latin letters about 0.56em. Boxes are sized from these before dagre places them.
 */

const MONO_ADVANCE = 0.6;
const LATIN_ADVANCE = 0.56;

const HAN = /\p{Script=Han}|[　-〿＀-￯]/u;

/** The width of a text in the monospaced face at a size; Han glyphs, as in a summary, count a full em. */
export function monoWidth(text: string, size: number): number {
  let width = 0;
  for (const glyph of text) {
    width += HAN.test(glyph) ? size : size * MONO_ADVANCE;
  }
  return width;
}

/** The width of a text in the text face at a size. */
export function textWidth(text: string, size: number): number {
  let width = 0;
  for (const glyph of text) {
    width += HAN.test(glyph) ? size : size * LATIN_ADVANCE;
  }
  return width;
}

/** How many lines a text wraps to in a width. */
export function linesOf(width: number, room: number): number {
  return Math.max(1, Math.ceil(width / room));
}

/** The sizes of the faces the drawings use. */
export const CODE_SIZE = 12;
export const MONO_SIZE = 11;
export const SMALL_SIZE = 13;
