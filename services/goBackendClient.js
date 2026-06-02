// services/goBackendClient.js
// Shared utility for Node.js → Go backend server-to-server calls.
// All direct calls to the Go API must include X-Internal-Proxy-Secret,
// otherwise the Go auth middleware rejects them with 403.

const axios = require('axios');

const GO_BACKEND_URL = process.env.GO_BACKEND_URL || `http://localhost:${process.env.API_PORT || 8080}`;

function internalHeaders(extra = {}) {
    const secret = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
    return {
        'Content-Type': 'application/json',
        ...(secret ? { 'X-Internal-Proxy-Secret': secret } : {}),
        ...extra,
    };
}

const goClient = axios.create({ baseURL: GO_BACKEND_URL, timeout: 15000 });

goClient.interceptors.request.use(config => {
    const secret = process.env.INTERNAL_PROXY_SECRET || process.env.JWT_SECRET || '';
    if (secret) config.headers['X-Internal-Proxy-Secret'] = secret;
    return config;
});

module.exports = { GO_BACKEND_URL, internalHeaders, goClient };
