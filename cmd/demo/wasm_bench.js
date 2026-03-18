// WASM performance benchmark — measures day-scaling for gobank simulation.
// Run: node wasm_bench.js <path-to-docs/demo>
// Requires: task docs:build (to produce main.wasm)

'use strict';

const fs = require('fs');
const path = require('path');
const { webcrypto } = require('crypto');

globalThis.crypto = webcrypto;

const demoDir = path.resolve(process.argv[2] || path.join(__dirname, '..', '..', 'docs', 'demo'));
require(path.join(demoDir, 'wasm_exec.js'));

async function loadWASM() {
    const go = new Go();
    const wasmPath = path.join(demoDir, 'main.wasm');
    const buf = fs.readFileSync(wasmPath);
    const sizeMB = (buf.length / 1024 / 1024).toFixed(1);

    return new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('wasmReady timeout')), 60000);
        globalThis.wasmReady = function () {
            clearTimeout(timeout);
            resolve(sizeMB);
        };
        WebAssembly.instantiate(buf, go.importObject).then(result => {
            go.run(result.instance);
        }).catch(reject);
    });
}

function benchDays(nCustomers, nDays) {
    goReset();
    goAddCustomers(nCustomers);

    const t0 = process.hrtime.bigint();
    for (let d = 0; d < nDays; d++) {
        goAdvanceDay();
    }
    const elapsedMs = Number(process.hrtime.bigint() - t0) / 1e6;
    const usPerDay = (elapsedMs * 1000) / nDays;
    return { elapsedMs, usPerDay };
}

(async function main() {
    const sizeMB = await loadWASM();
    console.log(`WASM loaded (${sizeMB} MB)\n`);

    // --- Day-scaling: 1 customer, increasing days ---
    console.log('=== Day scaling: 1 customer, 1+ accounts ===');
    console.log('  Days  |  Total ms  |  us/day');
    console.log('  ------|------------|--------');
    const dayCounts = [7, 30, 60, 180, 365];
    const results = [];
    for (const days of dayCounts) {
        const r = benchDays(1, days);
        console.log(`  ${String(days).padStart(5)} | ${r.elapsedMs.toFixed(1).padStart(10)} | ${r.usPerDay.toFixed(0).padStart(6)}`);
        results.push({ days, ...r });
    }

    // Check linearity: us/day should be roughly constant if scaling is linear
    const first = results[0].usPerDay;
    const last = results[results.length - 1].usPerDay;
    console.log(`\n  Scaling: ${(last / first).toFixed(2)}x cost/day increase (${results[0].days}d → ${results[results.length - 1].days}d)`);

    // --- Render cost ---
    console.log('\n=== Render cost (after 60 days, 1 customer) ===');
    goReset();
    goAddCustomers(1);
    for (let d = 0; d < 60; d++) goAdvanceDay();

    const renderFns = [
        ['dashboard', () => goRender()],
        ['P&L', () => goRenderPnL()],
        ['balance sheet', () => goRenderBalanceSheet()],
        ['customers', () => goRenderCustomers()],
        ['products', () => goRenderProducts('savings')],
        ['treasury', () => goRenderTreasuryCash()],
    ];
    for (const [name, fn] of renderFns) {
        fn(); // warmup
        const t0 = process.hrtime.bigint();
        for (let i = 0; i < 100; i++) fn();
        const ms = Number(process.hrtime.bigint() - t0) / 1e6;
        console.log(`  ${name.padEnd(15)} ${(ms / 100).toFixed(2)}ms/render`);
    }
})();
