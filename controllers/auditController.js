// controllers/auditController.js — Enterprise audit log query API
const express = require('express');
const router = express.Router();
const { requireAuth, requireAdmin } = require('../middleware/auth');
const userService = require('../services/userService');

// GET /api/audit-logs — Paginated, filterable audit log viewer (admin only)
router.get('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        const AuditLog = userService.database.models.AuditLog;
        const { Op } = require('sequelize');

        const {
            userId,
            action,
            riskLevel,
            from,
            to,
            page = 1,
            limit = 50,
            export: doExport
        } = req.query;

        const where = {};
        if (userId) where.user_id = userId;
        if (action) where.action = { [Op.iLike]: `%${action}%` };
        if (riskLevel) where.risk_level = riskLevel;
        if (from || to) {
            where.created_at = {};
            if (from) where.created_at[Op.gte] = new Date(from);
            if (to) where.created_at[Op.lte] = new Date(to);
        }

        const pageNum = Math.max(1, parseInt(page, 10));
        const pageLimit = Math.min(500, Math.max(1, parseInt(limit, 10)));
        const offset = (pageNum - 1) * pageLimit;

        const { count, rows } = await AuditLog.findAndCountAll({
            where,
            order: [['created_at', 'DESC']],
            limit: pageLimit,
            offset
        });

        // CSV export
        if (doExport === 'csv') {
            const header = 'id,user_id,action,entity_type,entity_id,ip_address,result,risk_level,created_at\n';
            const lines = rows.map(r =>
                [r.id, r.user_id || '', r.action, r.entity_type || '', r.entity_id || '',
                 r.ip_address || '', r.result || '', r.risk_level || '',
                 r.created_at ? r.created_at.toISOString() : ''].join(',')
            ).join('\n');
            res.setHeader('Content-Type', 'text/csv');
            res.setHeader('Content-Disposition', 'attachment; filename="audit-logs.csv"');
            return res.send(header + lines);
        }

        res.json({
            logs: rows,
            total: count,
            page: pageNum,
            totalPages: Math.ceil(count / pageLimit),
            limit: pageLimit
        });
    } catch (error) {
        console.error('❌ Audit log query error:', error);
        res.status(500).json({ message: 'Failed to fetch audit logs' });
    }
});

module.exports = router;
