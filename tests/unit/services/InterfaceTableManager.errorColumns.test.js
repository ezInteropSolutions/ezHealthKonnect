// tests/unit/services/InterfaceTableManager.errorColumns.test.js
//
// Regression: the standard message-table schema omitted the error-capture columns that V23's
// add_error_to_message_stack() / get_message_errors() and the Go ErrorCaptureService depend on, so on 2,195 of 2,199
// interface tables error details were silently dropped and the message viewer's Errors endpoint answered 500
// ('column "error_stack" does not exist'). The standard schema now carries them, for new tables AND for the lazy
// upgrade of existing ones.
const mockQuery = jest.fn();
jest.mock('../../../config/database', () => ({ sequelize: { query: (...a) => mockQuery(...a) } }));

const tableManager = require('../../../services/InterfaceTableManager');

const ERROR_COLUMNS = ['error_stack', 'last_error_timestamp', 'last_error_stage', 'error_severity'];

describe('error-capture columns are part of the standard message-table schema', () => {
    beforeEach(() => mockQuery.mockReset());

    it.each(ERROR_COLUMNS)('new tables are created with %s', (col) => {
        expect(tableManager.getMessageTableSchema()).toMatch(new RegExp(`\\b${col}\\b`));
    });

    it('error_stack defaults to an empty JSON array so appending to it never yields NULL', () => {
        expect(tableManager.getMessageTableSchema()).toMatch(/error_stack\s+JSONB\s+NOT NULL\s+DEFAULT\s+'\[\]'::jsonb/i);
    });

    it.each(ERROR_COLUMNS)('%s is a required column', (col) => {
        expect(tableManager.REQUIRED_COLUMNS).toContain(col);
    });

    it('an existing table missing them is upgraded in place (ALTER TABLE … ADD COLUMN IF NOT EXISTS)', async () => {
        mockQuery.mockResolvedValue([]);
        await tableManager.migrateTableSchema('messages_intf_x', ERROR_COLUMNS);
        expect(mockQuery).toHaveBeenCalledTimes(1);
        const sql = String(mockQuery.mock.calls[0][0]);
        for (const col of ERROR_COLUMNS) expect(sql).toMatch(new RegExp(`ADD COLUMN IF NOT EXISTS ${col}\\b`));
        expect(sql).toMatch(/error_stack JSONB NOT NULL DEFAULT '\[\]'::jsonb/i);
    });

    it('the schema version is bumped so the change is visible', () => {
        expect(parseFloat(tableManager.CURRENT_SCHEMA_VERSION)).toBeGreaterThanOrEqual(1.3);
    });
});
