// WASM performance benchmark — measures day-scaling for the gobank
// simulation through the handler the service worker serves.
// Run: node wasm_bench.js <path-to-docs/demo>
// Requires: task docs:build (to produce main.wasm)

'use strict';

const { loadWASM } = require('./wasm_harness.js');

async function benchDays(demo, nCustomers, nDays) {
    await demo.post('/reset');
    await demo.post('/settings', { max_customers: nCustomers, day_length: '' }); // cap MaxCustomers to prevent unbounded generation
    await demo.addCustomers(nCustomers);

    const t0 = process.hrtime.bigint();
    for (let d = 0; d < nDays; d++) {
        await demo.post('/advance');
    }
    const elapsedMs = Number(process.hrtime.bigint() - t0) / 1e6;
    const usPerDay = (elapsedMs * 1000) / nDays;
    return { elapsedMs, usPerDay };
}

(async function main() {
    const demo = await loadWASM();
    console.log('WASM loaded\n');

    // --- Day-scaling: 1 customer, increasing days ---
    console.log('=== Day scaling: 1 customer, 1+ accounts ===');
    console.log('  Days  |  Total ms  |  us/day');
    console.log('  ------|------------|--------');
    const dayCounts = [7, 30, 60, 180, 365];
    const results = [];
    for (const days of dayCounts) {
        const r = await benchDays(demo, 1, days);
        results.push({ days, ...r });
        console.log('  ' + String(days).padStart(4) + '  |  ' + r.elapsedMs.toFixed(1).padStart(8) + '  |  ' + r.usPerDay.toFixed(0).padStart(6));
    }
    // Check linearity: us/day should be roughly constant if scaling is linear.
    // Compare 60d vs 365d (skip 7d — too short for stable measurement).
    const r60 = results.find(r => r.days === 60);
    const r365 = results.find(r => r.days === 365);
    const ratio = r365.usPerDay / r60.usPerDay;
    console.log('\n  Scaling: ' + ratio.toFixed(2) + 'x cost/day increase (60d -> 365d)');
    if (ratio > 2.0) {
        console.error('  WARN: non-linear scaling detected (ratio ' + ratio.toFixed(2) + ' > 2.0)');
        process.exitCode = 1;
    } else {
        console.log('  OK: scaling is near-linear');
    }

    // --- Render cost ---
    console.log('\n=== Render cost (after 60 days, 1 customer) ===');
    await benchDays(demo, 1, 60);
    for (const [name, page] of [['dashboard', '/'], ['P&L', '/accounting/pnl'], ['balance sheet', '/accounting/balance-sheet'],
        ['customers', '/customers'], ['products', '/products/savings'], ['treasury', '/treasury/cash']]) {
        await demo.get(page); // warmup
        const t0 = process.hrtime.bigint();
        for (let i = 0; i < 100; i++) await demo.get(page);
        const ms = Number(process.hrtime.bigint() - t0) / 1e6;
        console.log('  ' + name.padEnd(15) + ' ' + (ms / 100).toFixed(2) + 'ms/render');
    }
})();
