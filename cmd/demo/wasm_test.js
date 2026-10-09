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

// The staff web needs a login (story 1.7.1). In the tab the first admin's
// password is the demo one, which the login page states; the tab keeps
// the session cookie, as it does the customer app's.
async function testStaffLogin(demo) {
    console.log('\n--- Staff login (one session per tab) ---');
    let res = await demo.get('/customers');
    assert(res.status === 303 && res.location === Demo.scope + 'login?redirect=customers', 'a page without a session goes to the login page (got ' + res.status + ' ' + res.location + ')');
    res = await demo.get('/login');
    assertPage(res, '/login');
    assert(res.body.includes('the password is demo'), 'login page states the demo password');
    assert(!res.body.includes('href="customers"'), 'signed out, the navigation is not shown');
    res = await demo.post('/login', { login: 'admin', password: 'wrong', redirect: '' });
    assert(res.status === 401, 'a wrong password is refused (got ' + res.status + ')');
    res = await demo.post('/login', { login: 'admin', password: 'demo', redirect: 'customers' });
    assert(res.status === 303 && res.location === Demo.scope + 'customers', 'login returns to the page (got ' + res.status + ' ' + res.location + ')');
    res = await demo.get('/');
    assertPage(res, '/ signed in');
    assert(res.body.includes('<span class="signed-in">admin'), 'the layout shows who is signed in');
}

async function testStaffLogout(demo) {
    console.log('\n--- Staff logout ---');
    let res = await demo.post('/logout');
    assert(res.status === 303 && res.location === Demo.scope + 'login', 'logout returns to the login page (got ' + res.status + ' ' + res.location + ')');
    res = await demo.get('/');
    assert(res.status === 303, 'the staff session is gone after logout');
}

async function testInitialRender(demo) {
    console.log('\n--- Initial render (day 0, 0 customers) ---');
    for (const page of ['/', '/accounting/pnl', '/accounting/balance-sheet', '/customers', '/payments',
        '/settings', '/about', '/products/savings', '/products/lending', '/treasury/cash',
        '/treasury/capital', '/treasury/gilts', '/about/models', '/about/runtime', '/v1/screen/login']) {
        assertPage(await demo.get(page), page);
    }
    const dash = await demo.get('/');
    assert(dash.body.includes('>Run<'), 'not running initially');
}

async function testShortRun(demo, nCustomers, nDays) {
    console.log('\n--- Short run: ' + nCustomers + ' customers, ' + nDays + ' days ---');
    await demo.post('/reset');
    // A reset is a new run: its users and sessions go with the old one,
    // so the operator signs in again.
    let res = await demo.get('/');
    assert(res.status === 303 && res.location === Demo.scope + 'login', 'after a reset the staff session is gone (got ' + res.status + ' ' + res.location + ')');
    res = await demo.post('/login', { login: 'admin', password: 'demo', redirect: '' });
    assert(res.status === 303 && res.location === Demo.scope, 'the admin signs in to the new run (got ' + res.status + ' ' + res.location + ')');
    await demo.post('/settings', { max_customers: nCustomers, day_length: '' });
    await demo.addCustomers(nCustomers);
    assertPage(await demo.get('/customers'), 'customers after add');

    for (let d = 0; d < nDays; d++) {
        res = await demo.post('/advance');
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

// The customer web is the BFF's HTML (ADR-0002 stage 6, story 1.6.2). In
// the tab a service worker's response cannot set a cookie, so the demo
// keeps the session itself; the login screen states the demo password.
async function testCustomerWeb(demo) {
    console.log('\n--- Customer web (BFF HTML in the tab) ---');
    let res = await demo.get('/v1/screen/accounts');
    assert(res.status === 303 && res.location === Demo.scope + 'v1/screen/login', 'accounts without a session goes to the login screen (got ' + res.status + ' ' + res.location + ')');
    res = await demo.get('/v1/screen/login');
    assert(res.body.includes('the password is demo'), 'login screen states the demo password');
    res = await demo.post('/v1/login', { customer_id: 'cust-001', password: 'demo' });
    assert(res.status === 303 && res.location === Demo.scope + 'v1/screen/accounts', 'login lands on accounts (got ' + res.status + ' ' + res.location + ')');
    res = await demo.get('/v1/screen/accounts');
    assertPage(res, 'accounts with the tab\'s session');
    assert(res.body.includes('cust-001') && res.body.includes('Net Balance'), 'accounts shows the customer\'s figures');
    for (const gone of ['/app/', '/api/customers', '/api/customer/cust-001/accounts']) {
        res = await demo.get(gone);
        assert(res.status === 404, gone + ' is retired (got ' + res.status + ')');
    }
    res = await demo.post('/v1/logout');
    assert(res.status === 303 && res.location === Demo.scope + 'v1/screen/login', 'logout returns to the login screen');
    res = await demo.get('/v1/screen/accounts');
    assert(res.status === 303, 'the session is gone after logout');
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

    await testStaffLogin(demo);
    await testInitialRender(demo);
    await testShortRun(demo, 10, 7);
    await testPayments(demo);
    await testCustomerWeb(demo);
    await testExport(demo);
    await testStaffLogout(demo);

    console.log('\n=== Results: ' + passes + ' passed, ' + failures + ' failed ===');
    process.exit(failures > 0 ? 1 : 0);
})();
