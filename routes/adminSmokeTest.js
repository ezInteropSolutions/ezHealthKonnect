'use strict';
/**
 * Admin Smoke Test route — streams live results via Server-Sent Events.
 *
 * GET  /api/admin/smoke-test/run
 *   Opens an SSE stream. One 'step' event per test step, one 'done' event
 *   at the end. Admin session required.
 *
 * GET  /api/admin/smoke-test/status
 *   Returns { running: bool } so the UI can check before opening the stream.
 */

const express = require('express');
const router  = express.Router();
const { SmokeTestRunner } = require('../services/smokeTestRunner');
const { requireAuth, requireAdmin } = require('../middleware/auth');

// Only one run at a time — prevent accidental concurrent executions.
let _running = false;

// ── Status ────────────────────────────────────────────────────────────────────

router.get('/status', requireAuth, requireAdmin, (_req, res) => {
    res.json({ running: _running });
});

// ── SSE run endpoint ──────────────────────────────────────────────────────────

router.get('/run', requireAuth, requireAdmin, (req, res) => {
    if (_running) {
        res.status(409).json({ error: 'A smoke test run is already in progress. Please wait for it to finish.' });
        return;
    }

    _running = true;

    // ── SSE headers ──────────────────────────────────────────────────────────
    res.writeHead(200, {
        'Content-Type':  'text/event-stream',
        'Cache-Control': 'no-cache, no-transform',
        'Connection':    'keep-alive',
        'X-Accel-Buffering': 'no',   // disable Nginx buffering if present
    });
    res.flushHeaders();

    const send = (event, data) => {
        if (res.writableEnded) return;
        res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
    };

    // Keep-alive ping every 15 s so the connection doesn't idle-timeout
    const ping = setInterval(() => {
        if (res.writableEnded) { clearInterval(ping); return; }
        res.write(': ping\n\n');
    }, 15000);

    // ── Build runner ─────────────────────────────────────────────────────────
    const baseURL = `http://localhost:${process.env.PORT || 3000}`;
    const secret  = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
    // Reuse the requesting admin's session cookie — no login step needed.
    const cookie  = req.headers.cookie || '';
    const runner  = new SmokeTestRunner(baseURL, secret, cookie);

    runner.on('step', step => send('step', step));

    runner.on('done', summary => {
        send('done', summary);
        clearInterval(ping);
        _running = false;
        res.end();
    });

    // Abort cleanly when the browser tab closes
    req.on('close', () => {
        runner.abort();
        clearInterval(ping);
        _running = false;
    });

    // Start — errors are caught inside runner and emitted as 'step' fail events
    runner.run().catch(err => {
        send('error', { message: err.message });
        clearInterval(ping);
        _running = false;
        res.end();
    });
});

module.exports = router;
