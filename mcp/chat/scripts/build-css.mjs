// Compile src/styles/island.css (Tailwind 4) to src/generated/island.css, ready to be adopted into a
// shadow root. Tailwind 4 emits things a shadow root cannot use or must not carry:
//
//   - `@property` registrations. A stylesheet inside a shadow tree cannot register custom properties,
//     so every `--tw-*` initial value would be missing and utilities such as shadows and rings would
//     compute to invalid. They are replaced by plain initial declarations on `*, ::before, ::after`,
//     which is what Tailwind's own non-@property fallback does.
//   - `:root` as a selector for theme variables. Inside a shadow tree `:root` matches nothing; `:host`
//     is the right anchor.
//
// The script then asserts the result holds nothing that could reach outside the shadow root or the
// network: no html/body/:root selector, no @font-face, no @import, no url().
import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import postcss from 'postcss';
import tailwindcss from '@tailwindcss/postcss';

const root = resolve(import.meta.dirname, '..');
const input = resolve(root, 'src/styles/island.css');
const output = resolve(root, 'src/generated/island.css');

/** Replace @property with initial declarations and :root with :host. */
function shadowRootPlugin() {
  return {
    postcssPlugin: 'shadow-root-css',
    OnceExit(css) {
      const initial = new Map();
      css.walkAtRules('property', (rule) => {
        const name = rule.params.trim();
        let value;
        rule.walkDecls('initial-value', (d) => {
          value = d.value;
        });
        if (value !== undefined) initial.set(name, value);
        rule.remove();
      });

      // Tailwind's own fallback for browsers without @property sits in `@layer properties` behind a
      // user-agent @supports test that is false in Chromium. Drop it and write the unconditional form.
      css.walkAtRules('layer', (rule) => {
        if (rule.params.trim() === 'properties' && rule.nodes) rule.remove();
      });
      if (initial.size > 0) {
        const block = postcss.atRule({ name: 'layer', params: 'properties' });
        const sel = postcss.rule({ selector: '*, ::before, ::after, ::backdrop' });
        for (const [prop, value] of initial) sel.append(postcss.decl({ prop, value }));
        block.append(sel);
        css.prepend(block);
      }

      css.walkRules((rule) => {
        rule.selectors = rule.selectors.map((s) => (s === ':root' ? ':host' : s.replace(/^:root\s*,\s*/, '')));
        rule.selector = [...new Set(rule.selectors)].join(', ');
      });
    },
  };
}
shadowRootPlugin.postcss = true;

// Tailwind's scanner reads every word in the source, comments and identifiers included, so words such as
// "shadow" (shadowRoot), "ring" (string) and "block" produce utilities nobody asked for, and some break
// the design (`.shadow` and `.ring` are box-shadows; DESIGN.md has none). Keep a utility only when a
// className attribute or cn() call in a .tsx file spells it out.
function classTokensOf(dir) {
  const out = [];
  const walk = (d) => {
    for (const name of readdirSync(d)) {
      const p = resolve(d, name);
      if (statSync(p).isDirectory()) walk(p);
      else if (p.endsWith('.tsx')) {
        const text = readFileSync(p, 'utf8');
        for (const m of text.matchAll(/className=(?:"([^"]*)"|\{`([^`]*)`\})|\bcn\(([^)]*)\)/g)) out.push(m[1] ?? m[2] ?? m[3] ?? '');
        for (const m of text.matchAll(/^.*\b(?:className|cn)\b.*$/gm)) out.push(m[0]);
      }
    }
  };
  walk(dir);
  return new Set(out.join(' ').split(/[\s'"`,]+/).filter(Boolean));
}

/** The class a utility rule's selector names, unescaped: .disabled\:opacity-45:disabled -> disabled:opacity-45 */
const classOfSelector = (selector) => {
  const m = /^\.((?:\\.|[^.:\s>+~[,])+)/.exec(selector);
  return m ? m[1].replace(/\\(.)/g, '$1') : undefined;
};

function pruneUnusedUtilities() {
  const used = classTokensOf(resolve(root, 'src'));
  return {
    postcssPlugin: 'prune-unused-utilities',
    OnceExit(css) {
      css.walkAtRules('layer', (layer) => {
        if (layer.params.trim() !== 'utilities') return;
        layer.walkRules((rule) => {
          if (rule.parent?.type !== 'root' && rule.parent !== layer) return; // keep rules nested in at-rules such as @media
          const name = classOfSelector(rule.selector);
          if (name !== undefined && !used.has(name)) rule.remove();
        });
      });
    },
  };
}
pruneUnusedUtilities.postcss = true;

const source = readFileSync(input, 'utf8');
const result = await postcss([tailwindcss({ optimize: false }), shadowRootPlugin(), pruneUnusedUtilities()]).process(source, { from: input });
// Keep Tailwind's MIT notice, drop the URL from the banner: the island ships no URL it does not use.
const css = result.css.replace(/(\/\*! tailwindcss v[\d.]+ \| MIT License) \| https:\/\/tailwindcss\.com( \*\/)/, '$1$2');

const FORBIDDEN = [
  [/@property\b/, '@property cannot be registered from a shadow tree'],
  [/@font-face\b/, '@font-face is ignored in a shadow root; fonts come from the library FontFace registration'],
  [/@import\b/, '@import would be a fetch'],
  [/\burl\(/, 'url() would be a fetch'],
  [/(^|[},\s])(html|body)\b[^{]*\{/m, 'html/body selectors restyle the host page'],
  [/:root\b/, ':root does not match inside a shadow tree'],
  [/::file-selector-button/, 'preflight leaked in'],
];
for (const [re, why] of FORBIDDEN) {
  const m = re.exec(css);
  if (m) {
    console.error(`build-css: forbidden ${JSON.stringify(m[0].trim())}: ${why}`);
    process.exit(1);
  }
}

mkdirSync(dirname(output), { recursive: true });
writeFileSync(output, css);
console.log(`build-css: wrote ${output} (${css.length} bytes)`);
