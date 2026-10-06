// WASM integration test — drives the gobank WASM binary the way the
// service worker does (ADR-0002 stage 6, story 1.6.1): the binary serves
// the demo's one handler through go-wasm-http-server, so the test hands it
// Requests and reads Responses, exactly what sw.js does in the browser.
// Run: node wasm_test.js <path-to-docs/demo>
// Exit 0 on success, 1 on failure.

'use strict';

const { loadWASM, Demo } = require('./wasm_harness.js');

let failures = 0;
let passes = 0;

function assert(cond, msg) {
    if (!cond) {
        console.error('  FAIL:', msg);
        failures++;
    } else {
        passes++;
    }
}

// assertPage asserts a GET answered 200 with a document inside the scope.
function assertPage(res, label) {
    assert(res.status === 200, label + ': status ' + res.status);
    assert(res.body.length > 10, label + ' returns non-trivial HTML (' + res.body.length + ' chars)');
    assert(res.body.includes('<base href="' + Demo.scope + '">'), label + ' carries the scope as its <base>');
}

async function testInitialRender(demo) {
    console.log('\n--- Initial render (day 0, 0 customers) ---');
    for (const page of ['/', '/accounting/pnl', '/accounting/balance-sheet', '/customers', '/payments',
        '/settings', '/about', '/products/savings', '/products/lending', '/treasury/cash',
        '/treasury/capital', '/treasury/gilts', '/about/models', '/about/runtime', '/app/']) {
        assertPage(await demo.get(page), page);
    }
    const dash = await demo.get('/');
    assert(dash.body.includes('>Run<'), 'not running initially');
}

async function testShortRun(demo, nCustomers, nDays) {
    console.log('\n--- Short run: ' + nCustomers + ' customers, ' + nDays + ' days ---');
    await demo.post('/reset');
    await demo.post('/settings', { max_customers: nCustomers, day_length: '' });
    await demo.addCustomers(nCustomers);
    assertPage(await demo.get('/customers'), 'customers after add');

    for (let d = 0; d < nDays; d++) {
        const res = await demo.post('/advance');
        assert(res.status === 303 && res.location === Demo.scope, 'advance redirects to the dashboard (got ' + res.status + ' ' + res.location + ')');
    }

    for (const page of ['/', '/accounting/pnl', '/accounting/balance-sheet', '/customers',
        '/treasury/cash', '/treasury/capital', '/reports/bbsi']) {
        assertPage(await demo.get(page), page + ' after ' + nDays + ' days');
    }

    // The explorer takes the request URL whole, so an FK link's filter
    // reaches it, and its links carry the scope.
    const filtered = await demo.get('/internal/explorer/customer_accounts?filter=customer_id&value=cust-001');
    assert(filtered.body.includes('Filter: customer_id = cust-001'), 'explorer FK filter honoured in WASM');
    assert(filtered.body.includes('href="' + Demo.scope + 'internal/explorer'), 'explorer links carry the scope');

    console.log('  ' + nDays + ' days advanced OK');
}

async function testPayments(demo) {
    console.log('\n--- Payments ---');
    for (let i = 0; i < 3; i++) {
        await demo.post('/payments/send');
    }
    assertPage(await demo.get('/payments'), 'payments after sends');
}

async function testRoleAndPII(demo) {
    console.log('\n--- Role and PII (one session per tab) ---');
    let res = await demo.post('/role', { role: 'readonly', redirect: 'customers' });
    assert(res.location === Demo.scope + 'customers', 'role change returns to the page (got ' + res.location + ')');
    res = await demo.get('/');
    assert(!res.body.includes('>Run<'), 'read-only role sees no simulation controls');
    await demo.post('/role', { role: 'admin', redirect: '' });
    res = await demo.get('/');
    assert(res.body.includes('>Run<'), 'admin role sees the simulation controls again');
}

async function testExport(demo) {
    console.log('\n--- Export ---');
    const res = await demo.get('/export.goluca');
    assert(res.status === 200, 'export answers 200 (got ' + res.status + ')');
    assert(res.body.length > 0, 'export produces data (' + res.body.length + ' bytes)');
    assert((res.headers.get('content-disposition') || '').includes('gobank.goluca'), 'export is an attachment');
}

(async function main() {
    let demo;
    try {
        demo = await loadWASM();
        console.log('WASM loaded OK, serving under ' + Demo.scope);
    } catch (err) {
        console.error('FATAL: WASM failed to load:', err.message);
        process.exit(1);
    }

    await testInitialRender(demo);
    await testShortRun(demo, 10, 7);
    await testPayments(demo);
    await testRoleAndPII(demo);
    await testExport(demo);

    console.log('\n=== Results: ' + passes + ' passed, ' + failures + ' failed ===');
    process.exit(failures > 0 ? 1 : 0);
})();
