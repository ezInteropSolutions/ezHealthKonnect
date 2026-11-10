// tests/unit/controllers/WizardMappingController.test.js
//
// Regression: the controller's `this.logger` was never assigned (the line was commented out), so the
// first statement of saveMappingConfiguration threw, the catch block threw again on the same logger,
// and the unhandled rejection killed the whole Node process — one authenticated POST to
// /api/wizard/save-mapping-config was a full outage (found by the v26.10 QA pass).
jest.mock('../../../services/WizardMappingService', () => jest.fn().mockImplementation(() => ({})));
jest.mock('../../../services/AccessPolicyService', () => ({ canSeeAllInterfaces: jest.fn().mockResolvedValue(true) }));

const WizardMappingController = require('../../../controllers/WizardMappingController');

function mockRes() {
    const res = {};
    res.status = jest.fn().mockReturnValue(res);
    res.json = jest.fn().mockReturnValue(res);
    return res;
}

describe('WizardMappingController logger', () => {
    it.each(['info', 'warn', 'error', 'debug'])('exposes a usable logger.%s', (level) => {
        const c = new WizardMappingController();
        expect(typeof c.logger[level]).toBe('function');
    });
});

describe('saveMappingConfiguration', () => {
    const req = (body) => ({ body, user: { id: 'u1', email: 'a@b.c' }, headers: {} });

    it('rejects an empty body with 400 instead of throwing', async () => {
        const c = new WizardMappingController();
        const res = mockRes();
        await expect(c.saveMappingConfiguration(req({}), res)).resolves.not.toThrow();
        expect(res.status).toHaveBeenCalledWith(400);
        expect(res.json.mock.calls[0][0]).toMatchObject({ success: false });
    });

    it('never rejects, even when the request itself is malformed', async () => {
        const c = new WizardMappingController();
        const res = mockRes();
        await expect(c.saveMappingConfiguration({ headers: {} }, res)).resolves.toBeDefined();
        expect(res.status).toHaveBeenCalledWith(500);
    });
});
