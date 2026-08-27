import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  // types.ts is generated from Go structs (see be/cmd/jobs.go) and
  // explicitly marked "do not change" - lint it as hand-written source and
  // a struct with no introspectable fields (e.g. an embedded time.Time)
  // produces an empty interface that trips no-empty-object-type, which
  // isn't a real code-quality issue to go fix by hand-editing generated
  // output.
  { ignores: ['dist', 'src/types.ts'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
    },
  },
);
