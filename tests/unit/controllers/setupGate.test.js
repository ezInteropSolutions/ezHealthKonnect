// tests/unit/controllers/setupGate.test.js
const { SetupGate } = require('../../../controllers/setupController');

describe('SetupGate (first-run setup token)', () => {
    it('requires the exact token when SETUP_TOKEN is configured', () => {
        const gate = new SetupGate({ SETUP_TOKEN: 'one-time-token-from-installer' });
        expect(gate.tokenRequired).toBe(true);
        expect(gate.verify('one-time-token-from-installer')).toBe(true);
        expect(gate.verify('one-time-token-from-installer ')).toBe(false);
        expect(gate.verify('wrong')).toBe(false);
        expect(gate.verify('')).toBe(false);
        expect(gate.verify(undefined)).toBe(false);
        expect(gate.verify({ token: 'one-time-token-from-installer' })).toBe(false);
    });

    it('stays open when no token is configured (local development)', () => {
        const gate = new SetupGate({});
        expect(gate.tokenRequired).toBe(false);
        expect(gate.verify(undefined)).toBe(true);
    });

    it('treats a blank SETUP_TOKEN as not configured', () => {
        expect(new SetupGate({ SETUP_TOKEN: '   ' }).tokenRequired).toBe(false);
    });
});
