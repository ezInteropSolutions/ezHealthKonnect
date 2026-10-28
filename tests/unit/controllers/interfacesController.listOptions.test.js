'use strict';
jest.mock('../../../config/database', () => ({}));
jest.mock('../../../services/AccessPolicyService', () => ({ canSeeAllInterfaces: jest.fn().mockResolvedValue(true) }));
jest.mock('../../../services/auditService', () => ({ logEvent: jest.fn().mockResolvedValue(undefined) }));

const controller = require('../../../controllers/interfacesController');

describe('interfaces list options (paging, search, light rows)', () => {
    const parse = (q) => controller.parseListOptions(q);

    it('no options = the full list, as before', () => {
        expect(parse({})).toEqual({ limit: null, offset: null, search: null, status: null, summary: false });
    });
    it('limit is clamped to 1..500 and offset never negative; offset needs a limit', () => {
        expect(parse({ limit: '25', offset: '50' })).toMatchObject({ limit: 25, offset: 50 });
        expect(parse({ limit: '99999' }).limit).toBe(500);
        expect(parse({ limit: '-3' }).limit).toBeNull();
        expect(parse({ limit: 'abc', offset: '10' })).toMatchObject({ limit: null, offset: null });
        expect(parse({ limit: '10', offset: '-5' }).offset).toBe(0);
    });
    it('search is trimmed and capped; status must look like a status word; view=summary is exact', () => {
        expect(parse({ q: '  abc  ' }).search).toBe('abc');
        expect(parse({ q: 'x'.repeat(500) }).search.length).toBe(100);
        expect(parse({ q: '   ' }).search).toBeNull();
        expect(parse({ status: "active'; DROP" }).status).toBeNull();
        expect(parse({ status: 'active' }).status).toBe('active');
        expect(parse({ view: 'summary' }).summary).toBe(true);
        expect(parse({ view: 'full' }).summary).toBe(false);
        expect(parse({ q: ['a', 'b'] }).search).toBeNull();
    });
});
