const tsPlugin = require("@typescript-eslint/eslint-plugin");
const tsParser = require("@typescript-eslint/parser");
const prettierRecommended = require("eslint-plugin-prettier/recommended");

module.exports = [
  {
    ignores: ["node_modules/**", "dist/**", "public/**", "src/types/proto/**"],
  },
  ...tsPlugin.configs["flat/recommended"],
  prettierRecommended,
  {
    files: ["src/**/*.{js,ts,tsx}"],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
        ecmaVersion: "latest",
        sourceType: "module",
      },
    },
    rules: {
      "prettier/prettier": ["error", { endOfLine: "auto" }],
      "@typescript-eslint/no-explicit-any": "off",
    },
  },
];
