// The viewer's own lint rules, for conventions oxlint's built-in rules cannot state; .oxlintrc.json loads them.

/** Han characters: the page's words, which only the term table and the kit's word list hold. */
const HAN = /\p{Script=Han}/u;

/** A string that starts like a page path, such as "/view"; the root "/" alone is not one. */
const PAGE_PATH = /^\/[a-z]/;

/** The elements a page never writes itself: every button and field is one of the design kit's controls. */
const KIT_ELEMENTS = new Set(['button', 'input', 'textarea', 'select']);

/** Arrows, crosses and the like, which a page draws as icons rather than typing them. */
const TYPED_SYMBOLS = /[→←‹›×−⋯↗]/u;


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
    'no-typed-symbols': {
      meta: { type: 'problem', docs: { description: 'Arrows, crosses and the like are Lucide icons drawn with the kit\'s Icon, never typed characters.' } },
      create(context) {
        const report = node => context.report({ node, message: 'Draw arrows, crosses and the like with the kit\'s Icon and a glyph from components/marks/icons (src/ui imports Lucide itself): a typed character takes the font\'s shape and weight, not the line icons\', and moves with neither the text colour nor the hover.' });
        return {
          JSXText(node) {
            if (TYPED_SYMBOLS.test(node.value)) {
              report(node);
            }
          },
          Literal(node) {
            if (typeof node.value === 'string' && TYPED_SYMBOLS.test(node.value)) {
              report(node);
            }
          },
          TemplateElement(node) {
            if (TYPED_SYMBOLS.test(node.value.cooked ?? '')) {
              report(node);
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
            if (node.source.value === 'lucide-react' || node.source.value.startsWith('lucide-react/')) {
              context.report({ node, message: 'Import Lucide glyphs from components/marks/icons, which names each glyph the viewer draws after docs/design.md §7.5\'s icon table; add a glyph there, and to the table, when a new meaning needs one.' });
            }
          },
        };
      },
    },
    'no-native-tooltip': {
      meta: {
        type: 'problem',
        docs: { description: 'Details wait in the kit\'s Hint, never in the browser\'s title tooltip.' },
        schema: [{ type: 'object', properties: { headings: { type: 'array', items: { type: 'string' } } }, required: ['headings'], additionalProperties: false }],
      },
      create(context) {
        // The components whose `title` is the text of a heading they draw, named in .oxlintrc.json; any other element's title is a tooltip.
        const headings = new Set(context.options[0].headings);
        return {
          JSXAttribute(node) {
            const element = node.parent;
            const name = element.name.type === 'JSXIdentifier' ? element.name.name : undefined;
            if (node.name.type === 'JSXIdentifier' && node.name.name === 'title' && !headings.has(name)) {
              context.report({ node, message: 'Put the detail in a Hint from src/ui/hint instead of a title: the browser\'s tooltip has neither the kit\'s look nor a reliable delay, and touch and keyboard readers never see it. A component whose title is a heading\'s text is named in the rule\'s headings in .oxlintrc.json.' });
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
