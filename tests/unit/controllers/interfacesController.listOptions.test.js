'use strict';
jest.mock('../../../config/database', () => ({}));
jest.mock('../../../services/AccessPolicyService', () => ({ canSeeAllInterfaces: jest.fn().mockResolvedValue(true) }));
jest.mock('../../../services/auditService', () => ({ logEvent: jest.fn().mockResolvedValue(undefined) }));

const controller = require('../../../controllers/interfacesController');

describe('interfaces list options (paging, search, light rows)', () => {
    const parse = (q) => controller.parseListOptions(q);

    it('no options = the full list, as before', () => {
        expect(parse({})).toEqual({ limit: null, offset: null, search: null, status: null, sourceType: null, format: null, summary: false, counts: false, orderBy: 'i.updated_at DESC NULLS LAST' });
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

describe('interfaces list: sort, filters and summary counts options', () => {
    const parse = (q) => controller.parseListOptions(q);

    it('the sort key comes from a fixed table - an unknown key falls back, nothing is interpolated', () => {
        expect(parse({}).orderBy).toBe('i.updated_at DESC NULLS LAST');
        expect(parse({ sort: 'name', dir: 'asc' }).orderBy).toBe('LOWER(i.name) ASC NULLS LAST');
        expect(parse({ sort: 'statistics', dir: 'desc' }).orderBy).toBe('COALESCE(i.total_processed, 0) DESC NULLS LAST');
        expect(parse({ sort: 'lastActivity', dir: 'asc' }).orderBy).toBe('i.last_processed_at ASC NULLS LAST');
        const evil = parse({ sort: 'name; DROP TABLE interfaces;--', dir: "asc'; --" }).orderBy;
        expect(evil).toBe('i.updated_at DESC NULLS LAST');
        expect(parse({ sort: '__proto__', dir: 'asc' }).orderBy).toBe('i.updated_at ASC NULLS LAST');
        expect(parse({ sort: 'constructor' }).orderBy).toBe('i.updated_at DESC NULLS LAST');
    });

    it('status is lower-cased; type and format accept only word characters; counts is opt-in', () => {
        expect(parse({ status: 'ACTIVE' }).status).toBe('active');
        expect(parse({ status: "a'b" }).status).toBeNull();
        expect(parse({ sourceType: 'tcp' }).sourceType).toBe('tcp');
        expect(parse({ sourceType: "x'; --" }).sourceType).toBeNull();
        expect(parse({ format: 'HL7v2' }).format).toBe('hl7v2');
        expect(parse({ format: 'a b' }).format).toBeNull();
        expect(parse({ counts: '1' }).counts).toBe(true);
        expect(parse({}).counts).toBe(false);
    });
});
