// tests/unit/noCommittedSecrets.test.js
//
// Repository policy (2026-10-20): no secret value is ever committed, and no
// code falls back to a hardcoded secret. A dev APP_CREDENTIAL_KEY and the dev
// session/JWT/database secrets sat in the public repo's docker-compose.yml for
// weeks; the generic gitleaks rule only caught the key. This test enforces the
// policy in `npm test` too, independent of the gitleaks CI job.
//
// Uses `git ls-files` so only committed/tracked files are checked (the local,
// gitignored .env legitimately holds secrets).
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..', '..');

const SECRET_NAMES = ['SESSION_SECRET', 'JWT_SECRET', 'INTERNAL_PROXY_SECRET', 'APP_CREDENTIAL_KEY',
    'DB_PASSWORD', 'MINIO_PASSWORD', 'OBJECT_STORAGE_SECRET_KEY', 'POSTGRES_PASSWORD',
    'MINIO_ROOT_PASSWORD', 'FLYWAY_PASSWORD', 'PGPASSWORD'];

// The one burned value CI's throwaway service container still uses — it is
// fingerprinted in config/compromised_secrets.json, so production refuses it.
const BURNED_CI_ONLY = ['secure_password_change_me'];

function trackedFiles() {
    return execSync('git ls-files', { cwd: ROOT, encoding: 'utf8' }).split('\n').filter(Boolean);
}

const files = trackedFiles();
const read = (f) => fs.readFileSync(path.join(ROOT, f), 'utf8');

describe('no committed secret values in deployment/config files', () => {
    const configLike = files.filter(f => /(\.ya?ml|\.env(\.[a-z0-9_-]+)?|Dockerfile[^/]*|\.sh|\.ps1|\.bat|\.cmd|\.conf|\.properties|\.toml|\.ini)$/i.test(f));
    const literal = new RegExp(`\\b(${SECRET_NAMES.join('|')})\\b[ \\t]*[:=][ \\t]*["']?([^\\s"'$<{}#?-][^\\s"']{7,})`, 'i');

    it('finds the deployment files (sanity check)', () => {
        expect(configLike).toEqual(expect.arrayContaining(['docker-compose.yml', '.env.example']));
    });

    it.each(configLike.map(f => [f]))('%s gives no literal value to a secret variable', (f) => {
        const offenders = read(f).split(/\r?\n/)
            .map((line, i) => ({ line, n: i + 1, m: line.match(literal) }))
            .filter(({ line, m }) => m && !BURNED_CI_ONLY.some(v => line.includes(v)))
            .map(({ n, m }) => `line ${n}: ${m[1]} (use a \${${m[1]}} reference to .env)`);
        expect(offenders).toEqual([]);
    });
});

describe('no hardcoded fallback secrets in code', () => {
    const name = '[A-Z_]*(?:SECRET|PASSWORD|KEY|TOKEN)[A-Z_]*';
    const jsFallback = new RegExp(`process\\.env\\.${name}\\s*(?:\\|\\||\\?\\?)\\s*(['"\`])[^'"\`]+\\1`);
    const goFallback = new RegExp(`getEnv\\(\\s*"${name}"\\s*,\\s*"[^"]+"\\s*\\)`);

    const code = files.filter(f => !/^(tests|public|node_modules|third_party)\//.test(f) && !/_test\.go$/.test(f));

    it.each([
        ['JavaScript', code.filter(f => f.endsWith('.js')), jsFallback],
        ['Go', code.filter(f => f.endsWith('.go')), goFallback],
    ])('%s: no secret falls back to a literal value', (_, list, re) => {
        const offenders = [];
        for (const f of list) {
            read(f).split(/\r?\n/).forEach((line, i) => {
                if (re.test(line)) offenders.push(`${f}:${i + 1}`);
            });
        }
        expect(offenders).toEqual([]);
    });
});
