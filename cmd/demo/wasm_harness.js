// wasm_harness drives the gobank WASM binary the way the service worker
// does: it plays go-wasm-http-server's host (the `wasmhttp` global the
// binary looks for), receives the handler the binary registers, and makes
// requests of it with the web platform's Request and Response, which Node
// 18+ provides. The demo is served under Demo.scope, as it is on the docs
// site, so what the tests see is what the browser gets.
//
// Shared by wasm_test.js and wasm_bench.js.

'use strict';

const fs = require('fs');
const path = require('path');
const { webcrypto } = require('crypto');

if (typeof globalThis.crypto === 'undefined') {
    globalThis.crypto = webcrypto;
}

const demoDir = path.resolve(process.argv[2] || path.join(__dirname, '..', '..', 'docs', 'demo'));
require(path.join(demoDir, 'wasm_exec.js'));

// Go's wasm_exec.js calls process.exit(2) on a panic: report it as such.
const origExit = process.exit;
process.exit = function (code) {
    if (code === 2) {
        console.error('\nGo panic detected (exit code 2)');
        origExit.call(process, 1);
    }
    origExit.call(process, code);
};

const origin = 'http://wasm.test';

// Demo is the running binary: GET and POST under its scope.
class Demo {
    static scope = '/demo/';

    constructor(handler) {
        this.handler = handler;
    }

    async get(p) {
        return this.request(new Request(origin + Demo.scope + p.replace(/^\//, '')));
    }

    async post(p, form) {
        const body = new URLSearchParams(form || {}).toString();
        return this.request(new Request(origin + Demo.scope + p.replace(/^\//, ''), {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body,
        }));
    }

    async request(req) {
        const res = await this.handler(req);
        return {
            status: res.status,
            headers: res.headers,
            location: res.headers.get('location'),
            body: await res.text(),
        };
    }

    // addCustomers starts a batch and waits until the dashboard reports it
    // done (the batch runs in the background, as in the browser).
    async addCustomers(n, timeoutMs) {
        await this.post('/add-customers', { n });
        const deadline = Date.now() + (timeoutMs || 60000);
        while (Date.now() < deadline) {
            const res = await this.get('/dashboard/update');
            if (!res.body.includes('>Adding<')) return;
            await new Promise(r => setTimeout(r, 50));
        }
        throw new Error('adding ' + n + ' customers did not finish in time');
    }
}

// loadWASM runs the binary and resolves with a Demo once it has registered
// its handler.
async function loadWASM() {
    const wasmPath = path.join(demoDir, 'main.wasm');
    const buf = fs.readFileSync(wasmPath);
    console.log('Loading WASM (' + (buf.length / 1024 / 1024).toFixed(1) + ' MB)...');

    const handler = new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('handler not registered within 60s')), 60000);
        globalThis.wasmhttp = {
            path: Demo.scope,
            setHandler(h) { clearTimeout(timeout); resolve(h); },
        };
    });

    const go = new Go();
    const result = await WebAssembly.instantiate(buf, go.importObject);
    go.run(result.instance);
    return new Demo(await handler);
}

module.exports = { loadWASM, Demo };
