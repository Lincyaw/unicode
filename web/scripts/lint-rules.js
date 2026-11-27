// The viewer's own lint rules, for conventions oxlint's built-in rules cannot state; .oxlintrc.json loads them.

/** Han characters: the page's words, which only the term table and the kit's word list hold. */
const HAN = /\p{Script=Han}/u;

/** A string that starts like a page path, such as "/view"; the root "/" alone is not one. */
const PAGE_PATH = /^\/[a-z]/;

/** The elements a page never writes itself: every button and field is one of the design kit's controls. */
const KIT_ELEMENTS = new Set(['button', 'input']);

/** A JSX element written in lower case is a DOM element rather than a component. */
const DOM_ELEMENT = /^[a-z]/;

export default {
  meta: { name: 'viewer' },
  rules: {
    'english-comments': {
      meta: { type: 'suggestion', docs: { description: 'Comments are written in English; the words readers see live in the term table.' } },
      create(context) {
        return {
          Program() {
            for (const comment of context.sourceCode.getAllComments()) {
              if (HAN.test(comment.value)) {
                context.report({ loc: comment.loc, message: 'Write comments in English; the page\'s Chinese words belong in src/terms.ts.' });
              }
            }
          },
        };
      },
    },
    'words-from-terms': {
      meta: { type: 'problem', docs: { description: 'Every Chinese word a page shows comes from the term table, so one concept has one word everywhere.' } },
      create(context) {
        const report = node => context.report({ node, message: 'Take the words a page shows from src/terms.ts (docs/design.md §7.5\'s term table), so each concept keeps one formal word on every page.' });
        return {
          Literal(node) {
            if (typeof node.value === 'string' && HAN.test(node.value)) {
              report(node);
            }
          },
          TemplateLiteral(node) {
            if (node.quasis.some(quasi => HAN.test(quasi.value.cooked ?? ''))) {
              report(node);
            }
          },
          JSXText(node) {
            if (HAN.test(node.value)) {
              report(node);
            }
          },
        };
      },
    },
    'controls-from-kit': {
      meta: { type: 'problem', docs: { description: 'Every button and field is a control of the design kit, so each has the kit\'s look, motion and focus ring.' } },
      create(context) {
        return {
          JSXOpeningElement(node) {
            if (node.name.type === 'JSXIdentifier' && KIT_ELEMENTS.has(node.name.name)) {
              context.report({ node, message: `Use a control of src/ui instead of a raw <${node.name.name}>: Button, IconButton or MarkButton for an action (MarkButton for a mark such as a node or a row), SearchField for a field. A look the kit lacks becomes a prop or variant in src/ui, noted in its README.` });
            }
          },
        };
      },
    },
    'icons-from-the-table': {
      meta: { type: 'problem', docs: { description: 'Every Lucide glyph a page draws is named in components/marks/icons, after the icon table of docs/design.md §7.5.' } },
      create(context) {
        return {
          ImportDeclaration(node) {
            if (node.source.value === 'lucide-react') {
              context.report({ node, message: 'Import Lucide glyphs from components/marks/icons, which names each glyph the viewer draws after docs/design.md §7.5\'s icon table; add a glyph there, and to the table, when a new meaning needs one.' });
            }
          },
        };
      },
    },
    'no-native-tooltip': {
      meta: { type: 'problem', docs: { description: 'Details wait in the kit\'s Hint, never in the browser\'s title tooltip.' } },
      create(context) {
        return {
          JSXAttribute(node) {
            const element = node.parent;
            if (node.name.type === 'JSXIdentifier' && node.name.name === 'title' && element.name.type === 'JSXIdentifier' && DOM_ELEMENT.test(element.name.name)) {
              context.report({ node, message: 'Put the detail in a Hint from src/ui/hint instead of a title attribute: the browser\'s tooltip has neither the kit\'s look nor a reliable delay, and touch and keyboard readers never see it.' });
            }
          },
        };
      },
    },
    'paths-from-routes': {
      meta: { type: 'problem', docs: { description: 'Every page path is built by src/routes, so a page or a component never spells one.' } },
      create(context) {
        const report = node => context.report({ node, message: 'Build page addresses with src/routes/address.ts, so a change of address happens in one place.' });
        return {
          Literal(node) {
            if (typeof node.value === 'string' && PAGE_PATH.test(node.value)) {
              report(node);
            }
          },
          TemplateLiteral(node) {
            if (PAGE_PATH.test(node.quasis[0].value.cooked ?? '')) {
              report(node);
            }
          },
        };
      },
    },
  },
};
