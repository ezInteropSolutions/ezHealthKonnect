// tests/unit/services/InterfaceTableManager.stats.test.js
//
// Regression (v26.10 UAT): the message page showed "Success Rate 0.0%" for an interface whose only message had been
// processed and delivered. getInterfaceTableStats counted only status 'sent'/'delivered' as successful, but the status
// a successfully processed message ENDS in is 'processed' (see the status CHECK constraint and the engine), so the
// rate was 0% for practically every interface.
const mockQuery = jest.fn();
jest.mock('../../../config/database', () => ({ sequelize: { query: (...a) => mockQuery(...a), QueryTypes: { SELECT: 'SELECT' } } }));

const tableManager = require('../../../services/InterfaceTableManager');

describe('getInterfaceTableStats', () => {
    let sql;
    beforeEach(async () => {
        mockQuery.mockReset();
        tableManager.checkTableExists = jest.fn().mockResolvedValue(true);
        mockQuery.mockResolvedValue([{ total_messages: '1', successful_messages: '1' }]);
        await tableManager.getInterfaceTableStats('11111111-2222-3333-4444-555555555555');
        sql = String(mockQuery.mock.calls[0][0]);
    });

    const clause = (alias) => {
        const m = new RegExp(`COUNT\\(CASE WHEN status IN \\(([^)]*)\\) THEN 1 END\\) as ${alias}`).exec(sql);
        return m ? m[1].replace(/['\s]/g, '').split(',') : [];
    };

    it.each(['processed', 'sent', 'delivered'])('counts %s as successful', (status) => {
        expect(clause('successful_messages')).toContain(status);
    });

    it.each(['failed', 'error'])('counts %s as failed', (status) => {
        expect(clause('failed_messages')).toContain(status);
    });

    it.each(['received', 'processing', 'transformed', 'reprocessing'])('counts %s as still processing', (status) => {
        expect(clause('processing_messages')).toContain(status);
    });

    it('never counts a status as both successful and failed', () => {
        const ok = clause('successful_messages');
        for (const s of clause('failed_messages')) expect(ok).not.toContain(s);
    });
});
