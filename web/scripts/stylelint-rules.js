// The viewer's own stylelint rule, for a convention stylelint's built-in rules cannot state; stylelint.config.js loads it.
import stylelint from 'stylelint';

const { createPlugin, utils: { report, ruleMessages, validateOptions } } = stylelint;

const ruleName = 'viewer/brand-only-for-focus';

const messages = ruleMessages(ruleName, {
  rejected: 'The brand colour (--accent-brand, --bg-brand-soft) is the focus ring\'s alone: use it only in a :focus-visible rule. Draw a chosen row with --row-chosen and a chosen chip or card with the static loop edge, and a status with its status colour.',
});

/** A declaration that reads the brand colour or its soft fill. */
const BRAND = /var\(\s*--(?:accent-brand|bg-brand-soft)\b/;

/** Reports every declaration that reads the brand colour in a rule some of whose selectors are not :focus-visible ones. */
function rule(primary) {
  return (root, result) => {
    if (!validateOptions(result, ruleName, { actual: primary })) {
      return;
    }
    root.walkDecls(decl => {
      const parent = decl.parent;
      const focused = parent?.type === 'rule' && parent.selectors.every(selector => selector.includes(':focus-visible'));
      if (BRAND.test(decl.value) && !focused) {
        report({ ruleName, result, node: decl, message: messages.rejected });
      }
    });
  };
}

rule.ruleName = ruleName;
rule.messages = messages;

export default createPlugin(ruleName, rule);
