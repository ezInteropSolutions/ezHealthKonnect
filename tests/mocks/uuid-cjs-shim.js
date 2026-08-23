// tests/mocks/uuid-cjs-shim.js
//
// The real `uuid` package ships as pure ESM ("type": "module" in its own
// package.json, e.g. node_modules/uuid/dist-node/index.js starts with
// `export { default as MAX } from './max.js';`). Production code's
// `require('uuid')` works today because current Node.js can transparently
// require() a synchronous ESM module — but Jest's own CommonJS-emulation
// module loader (jest-runtime) doesn't share that capability and fails to
// parse the `export` syntax. This shim is wired in via jest.config.js's
// moduleNameMapper so every test that transitively imports uuid-using code
// gets a real, working v4() without needing Babel/ESM transform config.
//
// Only `v4` is used anywhere in this codebase (confirmed via a full-repo
// grep) — crypto.randomUUID() produces the same RFC 4122 v4 UUID string
// shape, using Node's own built-in, no dependency needed.
const { randomUUID } = require('crypto');

module.exports = {
    v4: randomUUID,
};
