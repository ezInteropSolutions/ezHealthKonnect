// @ts-check
const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
    testDir: './tests/playwright',
    fullyParallel: false,
    retries: 1,
    workers: 1,
    timeout: 30_000,
    reporter: [
        ['list'],
        ['html', { open: 'never', outputFolder: 'tests/playwright-report' }],
        ['json', { outputFile: 'tests/playwright-report/results.json' }],
    ],
    use: {
        baseURL: process.env.BASE_URL || 'http://localhost:3000',
        trace: 'on-first-retry',
        screenshot: 'only-on-failure',
        video: 'off',
    },

    projects: [
        // ── Auth setup ─────────────────────────────────────────────────────────
        // Runs first, logs in once, saves browser storage state to .auth/user.json.
        // All other projects depend on this so they start already authenticated.
        // First-run setup through the real setup page with the installer's
        // one-time token. Only does anything on a fresh install (skips once
        // setup is complete), and must run before anything logs in.
        {
            name: 'first-run',
            testMatch: '**/first-run-setup.spec.js',
        },
        {
            name: 'setup',
            testMatch: '**/global-setup.spec.js',
            dependencies: ['first-run'],
        },

        // ── Desktop Chrome (primary) ───────────────────────────────────────────
        {
            name: 'chromium',
            use: {
                ...devices['Desktop Chrome'],
                storageState: '.auth/user.json',
            },
            dependencies: ['setup'],
            testIgnore: ['**/global-setup.spec.js', '**/first-run-setup.spec.js', '**/09-responsive.spec.js'],
        },

        // ── Firefox (cross-browser smoke) ──────────────────────────────────────
        {
            name: 'firefox',
            use: {
                ...devices['Desktop Firefox'],
                storageState: '.auth/user.json',
            },
            dependencies: ['setup'],
            // Only smoke tests — auth + dashboard + monitoring
            testMatch: ['**/01-auth.spec.js', '**/02-dashboard.spec.js', '**/05-monitoring.spec.js'],
        },

        // ── Mobile Chrome (responsive) ─────────────────────────────────────────
        {
            name: 'mobile-chrome',
            use: {
                ...devices['Pixel 7'],
                storageState: '.auth/user.json',
            },
            dependencies: ['setup'],
            testMatch: '**/09-responsive.spec.js',
        },

        // ── Tablet (responsive) ────────────────────────────────────────────────
        {
            name: 'tablet',
            use: {
                viewport: { width: 768, height: 1024 },
                userAgent: devices['Desktop Chrome'].userAgent,
                storageState: '.auth/user.json',
            },
            dependencies: ['setup'],
            testMatch: '**/09-responsive.spec.js',
        },
    ],
});
