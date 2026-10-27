'use strict';
/**
 * requireMessageContent — lets a request through only when the caller's role may read message CONTENT
 * (see services/AccessPolicyService: messageContent 'full' vs 'metadata'). Put it on every route that returns
 * what is inside a message (raw, transformed, parsed, per-step input/output); routes that return only details
 * (time, type, status, errors) do not need it.
 *
 * The refusal is a 403 with a stable `code`, so the UI can show a normal explanation instead of a failure.
 */

const accessPolicy = require('../services/AccessPolicyService');

const MESSAGE_CONTENT_RESTRICTED = 'MESSAGE_CONTENT_RESTRICTED';
const RESTRICTED_MESSAGE =
    'Your role can see message details (time, type, status and errors) but not message content. ' +
    'Ask an administrator if you need access.';

function requireMessageContent(policy = accessPolicy) {
    return async function messageContentGate(req, res, next) {
        try {
            const user = (req.session && req.session.user) || req.user;
            if (await policy.canViewMessageContent(user)) return next();
            return res.status(403).json({ success: false, code: MESSAGE_CONTENT_RESTRICTED, error: RESTRICTED_MESSAGE, message: RESTRICTED_MESSAGE });
        } catch (err) {
            console.error('❌ messageContentGate error:', err.message);
            // Fail closed: if the policy cannot be read, do not hand out patient content.
            return res.status(403).json({ success: false, code: MESSAGE_CONTENT_RESTRICTED, error: RESTRICTED_MESSAGE, message: RESTRICTED_MESSAGE });
        }
    };
}

module.exports = { requireMessageContent, MESSAGE_CONTENT_RESTRICTED, RESTRICTED_MESSAGE };
