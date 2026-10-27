// services/InterfaceCredentialMigrator.js
//
// One-time (idempotent) startup pass that encrypts connector secrets already
// stored in PLAINTEXT on interfaces rows — written before the interface config
// columns were protected at rest (2026-10-19). Covers source_config,
// target_config and the .config inside source_connectivity/target_connectivity,
// including soft-deleted rows (still data at rest). Runs only when
// APP_CREDENTIAL_KEY is configured: without a key there is nothing to encrypt
// with, and the startup warning in config/SecretsGuard.js already says so.
//
// Must run in Node (not a SQL migration): the AES key lives only in the app.

const CONFIG_COLUMNS = ['source_config', 'target_config'];
const CONNECTIVITY_COLUMNS = ['source_connectivity', 'target_connectivity'];

class InterfaceCredentialMigrator {
    /**
     * @param {object} deps
     * @param {object} deps.sequelize   Sequelize instance
     * @param {object} deps.protector   InterfaceConfigProtector
     * @param {object} [deps.logger]    console-like logger
     * @param {object} [deps.env]       environment (process.env)
     */
    constructor({ sequelize, protector, logger = console, env = process.env }) {
        this.sequelize = sequelize;
        this.protector = protector;
        this.logger = logger;
        this.env = env;
    }

    /** @returns {Promise<{scanned: number, updated: number, skipped: boolean}>} */
    async run() {
        if (!(this.env.APP_CREDENTIAL_KEY || '').trim()) {
            return { scanned: 0, updated: 0, skipped: true };
        }
        const rows = await this.sequelize.query(
            `SELECT id, ${CONFIG_COLUMNS.concat(CONNECTIVITY_COLUMNS).join(', ')} FROM interfaces`,
            { type: this.sequelize.QueryTypes.SELECT }
        );

        let updated = 0;
        for (const row of rows) {
            const changes = this.planRow(row);
            if (Object.keys(changes).length === 0) continue;
            const sets = Object.keys(changes).map(c => `${c} = :${c}::jsonb`).join(', ');
            const replacements = { id: row.id };
            for (const [c, v] of Object.entries(changes)) replacements[c] = JSON.stringify(v);
            await this.sequelize.query(`UPDATE interfaces SET ${sets} WHERE id = :id`, {
                replacements, type: this.sequelize.QueryTypes.UPDATE,
            });
            updated++;
        }
        if (updated > 0) {
            this.logger.log(`🔐 Encrypted plaintext connector secrets on ${updated} interface(s) (of ${rows.length} scanned)`);
        }
        return { scanned: rows.length, updated, skipped: false };
    }

    /**
     * Returns {column: newValue} for each column of row that holds a plaintext
     * secret; empty when nothing needs encrypting. Pure — no I/O.
     */
    planRow(row) {
        const changes = {};
        for (const column of CONFIG_COLUMNS) {
            const cfg = this._parse(row[column]);
            if (this.protector.hasPlaintextSecret(cfg)) {
                changes[column] = this.protector.protectForStorage(cfg, cfg);
            }
        }
        for (const column of CONNECTIVITY_COLUMNS) {
            const conn = this._parse(row[column]);
            if (conn && typeof conn === 'object' && this.protector.hasPlaintextSecret(conn.config)) {
                changes[column] = { ...conn, config: this.protector.protectForStorage(conn.config, conn.config) };
            }
        }
        return changes;
    }

    _parse(value) {
        if (value == null) return null;
        if (typeof value === 'object') return value;
        try { return JSON.parse(value); } catch (_) { return null; }
    }
}

module.exports = InterfaceCredentialMigrator;
