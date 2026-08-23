// jest.config.js
// Unit-test config for the Node.js layer. Deliberately scoped to tests/unit/
// only — kept separate from tests/playwright/ (browser E2E, its own runner)
// and the existing hand-rolled load/E2E scripts in tests/ (pas_template_test.js
// etc.), which are run directly via `node`, not Jest.
module.exports = {
    testEnvironment: 'node',
    testMatch: ['<rootDir>/tests/unit/**/*.test.js'],
    // uuid ships as pure ESM ("type": "module") — current Node.js can
    // require() it transparently, but Jest's CommonJS-emulation module
    // loader cannot. See tests/mocks/uuid-cjs-shim.js for the full
    // explanation; only v4() is used anywhere in this codebase.
    moduleNameMapper: {
        '^uuid$': '<rootDir>/tests/mocks/uuid-cjs-shim.js',
    },
    collectCoverageFrom: [
        'controllers/**/*.js',
        'services/**/*.js',
        'middleware/**/*.js',
    ],
    coverageDirectory: '<rootDir>/tests/coverage',
};
