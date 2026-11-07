import js from '@eslint/js';
import eslintPluginPrettierRecommended from 'eslint-plugin-prettier/recommended';
import eslintPluginSvelte from 'eslint-plugin-svelte';
import eslintPluginUnicorn from 'eslint-plugin-unicorn';
import { defineConfig } from 'eslint/config';
import globals from 'globals';
import svelteParser from 'svelte-eslint-parser';
import typescriptEslint from 'typescript-eslint';

export default defineConfig(
  { ignores: ['build', '.svelte-kit', 'node_modules', 'src/lib/kube/gen'] },
  js.configs.recommended,
  typescriptEslint.configs.recommendedTypeChecked,
  eslintPluginUnicorn.configs.recommended,
  ...eslintPluginSvelte.configs.recommended,
  {
    languageOptions: {
      globals: { ...globals.browser, ...globals.node },
      parserOptions: {
        projectService: { allowDefaultProject: ['static/*.js'] },
        extraFileExtensions: ['.svelte'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      curly: 'error',
      eqeqeq: 'error',
      'object-shorthand': ['error', 'always'],
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/require-await': 'error',
      '@typescript-eslint/switch-exhaustiveness-check': ['error', { considerDefaultExhaustiveForUnions: true }],
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_$', varsIgnorePattern: '^_$' }],
      'svelte/button-has-type': 'error',
      'svelte/no-at-const-tags': 'off',
      // As in Immich's web app.
      'unicorn/catch-error-name': 'off',
      'unicorn/class-reference-in-static-methods': ['error', { preferThis: false, preferSuper: false }],
      'unicorn/consistent-boolean-name': 'off',
      'unicorn/consistent-class-member-order': 'off',
      'unicorn/consistent-function-scoping': 'off',
      'unicorn/filename-case': 'off',
      'unicorn/import-style': 'off',
      'unicorn/max-nested-calls': 'off',
      'unicorn/name-replacements': 'off',
      'unicorn/no-array-callback-reference': 'off',
      'unicorn/no-asterisk-prefix-in-documentation-comments': 'off',
      'unicorn/no-break-in-nested-loop': 'off',
      'unicorn/no-declarations-before-early-exit': 'off',
      'unicorn/no-immediate-mutation': 'off',
      'unicorn/no-nested-ternary': 'off',
      'unicorn/no-non-function-verb-prefix': 'off',
      'unicorn/no-null': 'off',
      'unicorn/no-top-level-assignment-in-function': 'off',
      'unicorn/no-unreadable-for-of-expression': 'off',
      'unicorn/no-unsafe-property-key': 'off',
      'unicorn/no-useless-undefined': 'off',
      'unicorn/prefer-await': 'off',
      'unicorn/prefer-combined-guards': 'off',
      'unicorn/prefer-early-return': 'off',
      'unicorn/prefer-minimal-ternary': 'off',
      'unicorn/prefer-simple-condition-first': 'off',
      'unicorn/prefer-spread': 'off',
      'unicorn/prefer-ternary': 'off',
      'unicorn/prevent-abbreviations': 'off',
      'unicorn/single-line-block-comment-style': ['error', 'single-line'],
    },
  },
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: {
      parser: svelteParser,
      parserOptions: { parser: typescriptEslint.parser },
    },
  },
  eslintPluginPrettierRecommended,
);
