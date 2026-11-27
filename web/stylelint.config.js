/**
 * Tokens are the only source of visual truth: component CSS takes every colour, size and duration from the tokens
 * in src/ui, and every custom property it reads must be defined somewhere.
 */
const TOKEN_FILES = ['src/ui/tokens.css', 'src/ui/base.css'];

/** Properties a library sets on the elements it renders, for the stylesheet to read. */
const LIBRARY_PROPERTIES = {
  customProperties: {
    '--radix-select-trigger-width': '',
    '--radix-select-content-available-height': '',
    '--radix-popover-content-available-height': '',
    '--radix-popover-content-available-width': '',
  },
};

export default {
  extends: ['stylelint-config-standard'],
  plugins: ['stylelint-value-no-unknown-custom-properties', './scripts/stylelint-rules.js'],
  ignoreFiles: ['dist/**', 'node_modules/**'],
  rules: {
    'color-no-hex': true,
    'color-named': 'never',
    // px survives only as a media query breakpoint, because custom properties cannot be used inside @media.
    'unit-disallowed-list': [['px', 'ms', 's'], { ignoreMediaFeatureNames: { px: ['max-width', 'min-width'] } }],
    // A property a component's user or a script sets is a component parameter, read with a default as var(--name, value).
    'csstools/value-no-unknown-custom-properties': [true, { importFrom: [...TOKEN_FILES, LIBRARY_PROPERTIES] }],
    'viewer/brand-only-for-focus': true,
    'declaration-property-value-allowed-list': [{ 'font-weight': ['400', '500', '600'] }, { message: 'Use the weights 400, 500 and 600: PingFang SC, the Chinese face, goes no heavier than 600.' }],
    'declaration-property-value-disallowed-list': [{ 'font-style': ['italic', 'oblique'] }, { message: 'Interface text is never italic; emphasis comes from weight and colour.' }],
    'function-disallowed-list': [['cubic-bezier', 'steps'], { message: 'Take easing from the tokens: every change decelerates with --ease-out and never bounces.' }],
    'comment-pattern': [/^[^\p{Script=Han}]*$/u, { message: 'Write comments in English.' }],
    'selector-class-pattern': [
      /^(?!(?:active|selected|current|open|opened|expanded|collapsed|checked|pressed|chosen)$)/,
      { message: 'Style a state from its ARIA attribute (aria-pressed, aria-current, aria-expanded, aria-selected) or Radix\'s data-state, not from a class of its own.' },
    ],
    // Breakpoints are written the way the scripts' media queries are, as max-width.
    'media-feature-range-notation': null,
    // CSS modules scope their own class names, and a module reaches a drawing's plain classes through :global.
    'selector-pseudo-class-no-unknown': [true, { ignorePseudoClasses: ['global'] }],
    'custom-property-pattern': null,
    'keyframes-name-pattern': null,
    // The stylesheets keep short rules on one line.
    'declaration-block-single-line-max-declarations': null,
    'declaration-empty-line-before': null,
    'custom-property-empty-line-before': null,
    'rule-empty-line-before': null,
    'comment-empty-line-before': null,
    'at-rule-empty-line-before': null,
    'no-descending-specificity': null,
  },
  overrides: [
    {
      // The token files define the raw values everything else reads, and name the Figma collections they are exported from.
      files: TOKEN_FILES,
      rules: { 'color-no-hex': null, 'unit-disallowed-list': null, 'function-disallowed-list': null, 'comment-pattern': null },
    },
  ],
};
