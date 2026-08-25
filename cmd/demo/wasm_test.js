// WASM integration test — exercises the gobank WASM binary via Node.js.
// Run: node wasm_test.js <path-to-docs/demo>
// Exit 0 on success, 1 on failure.

'use strict';

const fs = require('fs');
const path = require('path');
const { webcrypto } = require('crypto');

// Polyfill for wasm_exec.js on older Node (modern Node has a getter-only
// globalThis.crypto that must not be assigned).
if (typeof globalThis.crypto === 'undefined') {
	globalThis.crypto = webcrypto;
}

const demoDir = path.resolve(process.argv[2] || path.join(__dirname, '..', '..', 'docs', 'demo'));
require(path.join(demoDir, 'wasm_exec.js'));

let failures = 0;
let passes = 0;

// Intercept Go panic exit — wasm_exec.js calls process.exit(2) on panic.
// Override to print results before dying.
const origExit = process.exit;
process.exit = function (code) {
    if (code === 2) {
        // Go panic — report what we have so far.
        console.error('\nGo panic detected (exit code 2)');
        console.log('\n=== Results (partial): ' + passes + ' passed, ' + (failures + 1) + ' failed ===');
        origExit.call(process, 1);
    }
    origExit.call(process, code);
};

function assert(cond, msg) {
    if (!cond) {
        console.error('  FAIL:', msg);
        failures++;
    } else {
        passes++;
    }
}

function assertHTML(val, label) {
    assert(typeof val === 'string' && val.length > 10, label + ' returns non-trivial HTML (got ' + (typeof val === 'string' ? val.length : typeof val) + ' chars)');
}

async function loadWASM() {
    const go = new Go();
    const wasmPath = path.join(demoDir, 'main.wasm');
    const buf = fs.readFileSync(wasmPath);
    console.log('Loading WASM (' + (buf.length / 1024 / 1024).toFixed(1) + ' MB)...');

    return new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('wasmReady not called within 60s')), 60000);

        globalThis.wasmReady = function () {
            clearTimeout(timeout);
            resolve();
        };

        WebAssembly.instantiate(buf, go.importObject).then(result => {
            go.run(result.instance);
        }).catch(reject);
    });
}

// --- Test suites ---

function testStartup() {
    console.log('\n--- Startup ---');
    // If we got here, wasmReady was called.
    assert(typeof globalThis.goRender === 'function', 'goRender exported');
    assert(typeof globalThis.goAddCustomers === 'function', 'goAddCustomers exported');
    assert(typeof globalThis.goAdvanceDay === 'function', 'goAdvanceDay exported');
    assert(typeof globalThis.goRenderPnL === 'function', 'goRenderPnL exported');
    assert(typeof globalThis.goRenderBalanceSheet === 'function', 'goRenderBalanceSheet exported');
    assert(typeof globalThis.goRenderCustomers === 'function', 'goRenderCustomers exported');
    assert(typeof globalThis.goRenderPayments === 'function', 'goRenderPayments exported');
    assert(typeof globalThis.goRenderSettings === 'function', 'goRenderSettings exported');
    assert(typeof globalThis.goRenderAbout === 'function', 'goRenderAbout exported');
    assert(typeof globalThis.goIsRunning === 'function', 'goIsRunning exported');
}

function testInitialRender() {
    console.log('\n--- Initial render (day 0, 0 customers) ---');
    assertHTML(goRender(), 'dashboard');
    assertHTML(goRenderPnL(), 'P&L');
    assertHTML(goRenderBalanceSheet(), 'balance sheet');
    assertHTML(goRenderCustomers(), 'customers');
    assertHTML(goRenderPayments(), 'payments');
    assertHTML(goRenderSettings(), 'settings');
    assertHTML(goRenderAbout(), 'about');
    assertHTML(goRenderProducts('savings'), 'savings products');
    assertHTML(goRenderProducts('lending'), 'lending products');
    assertHTML(goRenderTreasuryCash(), 'treasury cash');
    assertHTML(goRenderTreasuryCapital(), 'treasury capital');
    assertHTML(goRenderTreasuryGilts(), 'treasury gilts');
    assertHTML(goRenderModels(), 'models');
    assert(goIsRunning() === false, 'not running initially');
}

function testShortRun(nCustomers, nDays) {
    console.log('\n--- Short run: ' + nCustomers + ' customers, ' + nDays + ' days ---');
    goReset();
    goUpdateSettings(nCustomers); // fix customer count
    goAddCustomers(nCustomers);
    const custHTML = goRenderCustomers();
    assertHTML(custHTML, 'customers after add');

    for (let d = 0; d < nDays; d++) {
        goAdvanceDay();
    }

    const dash = goRender();
    assertHTML(dash, 'dashboard after ' + nDays + ' days');
    assertHTML(goRenderPnL(), 'P&L after ' + nDays + ' days');
    assertHTML(goRenderBalanceSheet(), 'balance sheet after ' + nDays + ' days');
    assertHTML(goRenderTreasuryCash(), 'treasury cash after ' + nDays + ' days');
    assertHTML(goRenderBBSI(), 'BBSI report after ' + nDays + ' days');

    console.log('  ' + nDays + ' days advanced OK');
}

function testFullYear(nCustomers) {
    console.log('\n--- Full year: ' + nCustomers + ' customers, 365 days ---');
    goReset();
    goUpdateSettings(nCustomers); // fix customer count
    goAddCustomers(nCustomers);

    // Time each half to detect non-linear scaling
    const t0 = Date.now();
    for (let d = 0; d < 182; d++) {
        goAdvanceDay();
    }
    const firstHalfMs = Date.now() - t0;

    const t1 = Date.now();
    for (let d = 182; d < 365; d++) {
        goAdvanceDay();
    }
    const secondHalfMs = Date.now() - t1;

    const totalMs = firstHalfMs + secondHalfMs;
    const ratio = secondHalfMs / firstHalfMs;

    const dash = goRender();
    assertHTML(dash, 'dashboard after 365 days');
    assertHTML(goRenderPnL(), 'P&L after 365 days');
    assertHTML(goRenderBalanceSheet(), 'balance sheet after 365 days');
    assertHTML(goRenderCustomers(), 'customers after 365 days');
    assertHTML(goRenderTreasuryCash(), 'treasury cash after 365 days');
    assertHTML(goRenderTreasuryCapital(), 'treasury capital after 365 days');
    assertHTML(goRenderBBSI(), 'BBSI after 365 days');

    console.log('  365 days in ' + (totalMs / 1000).toFixed(2) + 's (first half ' + firstHalfMs + 'ms, second half ' + secondHalfMs + 'ms, ratio ' + ratio.toFixed(2) + ')');
    // With linear scaling the second half should cost about the same as the first.
    // Allow up to 2x for WASM/GC variance; anything above indicates non-linear regression.
    assert(ratio < 2.0, 'day scaling is near-linear (second/first half ratio ' + ratio.toFixed(2) + ' < 2.0)');
}

function testPayments() {
    console.log('\n--- Payments ---');
    goResetPayments();
    goSendPayment();
    goSendPayment();
    goSendPayment();
    assertHTML(goRenderPayments(), 'payments after sends');
}

function testExportImport() {
    console.log('\n--- Export/Import ---');
    let data;
    try {
        data = goExport();
    } catch (err) {
        // Go panic kills the WASM instance; catch what we can.
        console.error('  FAIL: goExport() threw:', err.message || err);
        failures++;
        return;
    }
    if (data === undefined || data === null) {
        console.error('  FAIL: goExport() returned', data, '(likely Go panic — ledger not initialised)');
        failures++;
        return;
    }
    assert(typeof data === 'string' && data.length > 0, 'export produces data (' + data.length + ' bytes)');
    assert(!data.startsWith('error:'), 'export has no error: ' + data.substring(0, 80));
}

// --- Main ---

(async function main() {
    try {
        await loadWASM();
        console.log('WASM loaded OK');
    } catch (err) {
        console.error('FATAL: WASM failed to load:', err.message);
        process.exit(1);
    }

    testStartup();
    testInitialRender();
    testShortRun(10, 7);
    testPayments();
    testFullYear(1);
    // Export/Import last — depends on ledger which may not init in WASM yet.
    // A Go panic here kills the process, so keep it at the end.
    testExportImport();

    console.log('\n=== Results: ' + passes + ' passed, ' + failures + ' failed ===');
    process.exit(failures > 0 ? 1 : 0);
})();
