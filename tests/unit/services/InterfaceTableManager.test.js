// tests/unit/services/InterfaceTableManager.test.js
//
// InterfaceTableManager as a whole is DB-heavy and singleton-coupled, but
// getInterfaceTableName and buildFilterWhere are two genuinely pure,
// deterministic methods. config/database is mocked purely so requiring this
// module (a pre-instantiated singleton — module.exports = new
// InterfaceTableManager()) doesn't touch anything real; neither method under
// test reads it.
jest.mock('../../../config/database', () => ({}));

const tableManager = require('../../../services/InterfaceTableManager');

describe('getInterfaceTableName', () => {
    it('converts a UUID into a safe table identifier, using the FULL UUID', () => {
        const name = tableManager.getInterfaceTableName('629ac1e8-0c50-447a-b93f-ebfc15830a7d');
        expect(name).toBe('messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d');
    });

    it('is deterministic — same input always produces the same table name', () => {
        const id = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';
        expect(tableManager.getInterfaceTableName(id)).toBe(tableManager.getInterfaceTableName(id));
    });
});

describe('buildFilterWhere', () => {
    it('returns an empty WHERE clause and no replacements when called with nothing', () => {
        expect(tableManager.buildFilterWhere()).toEqual({ whereClause: '', replacements: {} });
        expect(tableManager.buildFilterWhere({})).toEqual({ whereClause: '', replacements: {} });
    });

    it('maps a known composite status to its SQL condition directly, without a replacement param', () => {
        const { whereClause, replacements } = tableManager.buildFilterWhere({ status: 'delivered' });
        expect(whereClause).toBe("WHERE status IN ('processed','delivered') AND delivery_status = 'delivered'");
        expect(replacements).toEqual({});
    });

    it('falls back to a plain status = :status replacement for an unrecognized status value', () => {
        const { whereClause, replacements } = tableManager.buildFilterWhere({ status: 'some_custom_status' });
        expect(whereClause).toBe('WHERE status = :status');
        expect(replacements).toEqual({ status: 'some_custom_status' });
    });

    it('wraps messageType in ILIKE %...% for partial matching', () => {
        const { whereClause, replacements } = tableManager.buildFilterWhere({ messageType: 'ADT' });
        expect(whereClause).toBe('WHERE message_type ILIKE :messageType');
        expect(replacements.messageType).toBe('%ADT%');
    });

    it('combines multiple filters with AND, in a fixed order', () => {
        const { whereClause, replacements } = tableManager.buildFilterWhere({
            status: 'failed',
            messageType: 'ORU',
            dateFrom: '2026-01-01',
            dateTo: '2026-01-31',
        });
        expect(whereClause).toBe(
            "WHERE (status = 'failed' OR delivery_status = 'failed') AND message_type ILIKE :messageType AND received_at >= :dateFrom AND received_at <= :dateTo"
        );
        expect(replacements).toEqual({ messageType: '%ORU%', dateFrom: '2026-01-01', dateTo: '2026-01-31' });
    });

    it.each([
        ['received', "status = 'received'"],
        ['processing', "status IN ('processing','reprocessing')"],
        ['completed', "status IN ('processed','delivered') AND delivery_status = 'not_required'"],
        ['pending_delivery', "status = 'processed' AND delivery_status = 'pending'"],
    ])('maps composite status "%s" correctly', (status, expectedCondition) => {
        const { whereClause } = tableManager.buildFilterWhere({ status });
        expect(whereClause).toBe(`WHERE ${expectedCondition}`);
    });
});
