// eslint.config.js — security lint for the browser UI only.
//
// One rule, deliberately: eslint-plugin-no-unsanitized fails any HTML sink
// (innerHTML, outerHTML, insertAdjacentHTML, document.write) built from an
// expression that does not go through public/js/core/html-safe.js. Style and
// other rules are out of scope here so this signal is never drowned out.
// Run: npm run lint:xss (CI runs it in the Node.js test job).
const noUnsanitized = require('eslint-plugin-no-unsanitized');
const html = require('eslint-plugin-html');

const escape = {
    taggedTemplates: ['html', 'HtmlSafe.html'],
    methods: ['escapeHtml', 'HtmlSafe.escapeHtml', 'HtmlSafe.raw', 'HtmlSafe.sanitize'],
};

module.exports = [
    {
        files: ['public/**/*.js', 'public/**/*.html'],
        ignores: ['public/vendor/**', 'public/**/*.min.js'],
        plugins: { 'no-unsanitized': noUnsanitized, html },
        languageOptions: { ecmaVersion: 2022, sourceType: 'script' },
        linterOptions: { reportUnusedDisableDirectives: 'off' },
        rules: {
            'no-unsanitized/property': ['error', { escape }],
            'no-unsanitized/method': ['error', { escape }],
        },
    },
    {
        files: ['public/js/modules/**/*.js'],
        languageOptions: { sourceType: 'module' },
    },
];
