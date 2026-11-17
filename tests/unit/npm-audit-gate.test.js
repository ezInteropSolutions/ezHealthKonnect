const { evaluate } = require('../../scripts/ci/npm-audit-gate');

const report = (...advs) => ({ vulnerabilities: Object.fromEntries(advs.map((a, i) => [`pkg${i}`, { via: [a, 'other'] }])) });
const adv = (id, severity, name = 'pkg') => ({ name, severity, title: 't', url: `https://github.com/advisories/${id}` });
const rule = (id, expires = '2027-01-01') => ({ id, package: 'pkg', reason: 'r', added: '2026-01-01', expires });

describe('npm audit gate', () => {
    test('passes when there is nothing', () => {
        expect(evaluate({ vulnerabilities: {} }, [], '2026-11-17')).toEqual({ failures: [], excepted: [], expired: [] });
    });
    test('fails on an unlisted moderate or worse advisory', () => {
        for (const sev of ['moderate', 'high', 'critical']) {
            expect(evaluate(report(adv('GHSA-aaaa', sev)), [], '2026-11-17').failures).toHaveLength(1);
        }
    });
    test('ignores low and info advisories', () => {
        expect(evaluate(report(adv('GHSA-low', 'low'), adv('GHSA-info', 'info')), [], '2026-11-17').failures).toHaveLength(0);
    });
    test('a listed advisory is excepted, and any other one still fails', () => {
        const r = evaluate(report(adv('GHSA-ok', 'high'), adv('GHSA-new', 'high')), [rule('GHSA-ok')], '2026-11-17');
        expect(r.excepted.map(a => a.id)).toEqual(['GHSA-ok']);
        expect(r.failures.map(a => a.id)).toEqual(['GHSA-new']);
    });
    test('an expired exception fails the gate until it is reviewed', () => {
        const r = evaluate(report(adv('GHSA-ok', 'high')), [rule('GHSA-ok', '2026-11-01')], '2026-11-17');
        expect(r.expired.map(a => a.id)).toEqual(['GHSA-ok']);
        expect(r.excepted).toHaveLength(0);
    });
    test('the same advisory reached through several packages counts once', () => {
        const rep = { vulnerabilities: { a: { via: [adv('GHSA-x', 'high')] }, b: { via: [adv('GHSA-x', 'high')] } } };
        expect(evaluate(rep, [], '2026-11-17').failures).toHaveLength(1);
    });
});
