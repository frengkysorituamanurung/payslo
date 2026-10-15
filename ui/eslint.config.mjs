import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import tseslint from 'typescript-eslint';

// Aturan lokal U0 (UI-SPEC no. 8): DILARANG string hardcoded di komponen.
// Setiap teks JSX harus lewat i18n (t("...")). Whitespace antar elemen
// dikecualikan. Hanya atribut yang terlihat pengguna yang dicek
// (aria-label, title, placeholder, alt); atribut teknis (type, role, to,
// name, ...) bebas. File test/setup/mocks dibebaskan.
const VISIBLE_ATTRS = new Set(['aria-label', 'aria-description', 'title', 'placeholder', 'alt']);
const noHardcodedString = {
    rules: {
        'no-hardcoded-string': {
            meta: { type: 'problem', docs: { description: 'UI text must go through i18n' } },
            create(context) {
                return {
                    JSXText(node) {
                        // Ikon simbolik (✓ ✕ ! ? ⏱) tanpa huruf = bukan teks bahasa.
                        if (node.value.trim() !== '' && /\p{L}/u.test(node.value)) {
                            context.report({
                                node,
                                message: 'Hardcoded UI string; use t("...") from i18n.',
                            });
                        }
                    },
                    JSXAttribute(node) {
                        if (
                            node.name.type === 'JSXIdentifier' &&
                            VISIBLE_ATTRS.has(node.name.name) &&
                            node.value &&
                            node.value.type === 'Literal' &&
                            typeof node.value.value === 'string'
                        ) {
                            context.report({
                                node,
                                message: 'Hardcoded JSX attribute string; use t("...").',
                            });
                        }
                    },
                };
            },
        },
    },
};

export default tseslint.config(
    { ignores: ['dist', 'src/api/schema.d.ts', 'playwright-report', 'test-results'] },
    {
        extends: [js.configs.recommended, ...tseslint.configs.strictTypeChecked],
        files: ['**/*.{ts,tsx}'],
        languageOptions: {
            ecmaVersion: 2022,
            globals: { ...globals.browser, ...globals.node },
            parserOptions: { projectService: true },
        },
        plugins: {
            'react-hooks': reactHooks,
            'react-refresh': reactRefresh,
            'jsx-a11y': jsxA11y,
            local: noHardcodedString,
        },
        rules: {
            ...reactHooks.configs.recommended.rules,
            ...jsxA11y.configs.recommended.rules,
            'react-refresh/only-export-components': 'warn',
            '@typescript-eslint/no-explicit-any': 'error',
            '@typescript-eslint/no-unused-vars': 'error',
        },
    },
    {
        files: ['src/**/*.{ts,tsx}'],
        ignores: ['src/**/*.test.{ts,tsx}', 'src/test/**', 'src/mocks/**'],
        rules: { 'local/no-hardcoded-string': 'error' },
    },
);
