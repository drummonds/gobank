const outputDiv = document.getElementById('output');
const statusTag = document.getElementById('status-tag');

// Bank controls
const controlsBank = document.getElementById('controls-bank');
const startStopBtn = document.getElementById('startStopBtn');
const advanceBtn = document.getElementById('advanceBtn');
const addCustBtn = document.getElementById('addCustBtn');
const addCustN = document.getElementById('addCustN');
const resetBtn = document.getElementById('resetBtn');
const exportBtn = document.getElementById('exportBtn');
const importFile = document.getElementById('importFile');

// Payment controls
const controlsPayments = document.getElementById('controls-payments');
const sendPaymentBtn = document.getElementById('sendPaymentBtn');
const autoPaymentsBtn = document.getElementById('autoPaymentsBtn');
const resetPaymentsBtn = document.getElementById('resetPaymentsBtn');

// Navigation elements
const navBrand = document.getElementById('nav-brand');
const navDashboard = document.getElementById('nav-dashboard');
const navPnl = document.getElementById('nav-pnl');
const navBalanceSheet = document.getElementById('nav-balance-sheet');
const navSavings = document.getElementById('nav-savings');
const navLending = document.getElementById('nav-lending');
const navCustomers = document.getElementById('nav-customers');
const navPayments = document.getElementById('nav-payments');
const navTreasuryCash = document.getElementById('nav-treasury-cash');
const navTreasuryCapital = document.getElementById('nav-treasury-capital');
const navTreasuryGilts = document.getElementById('nav-treasury-gilts');
const navSettings = document.getElementById('nav-settings');
const navExplorer = document.getElementById('nav-explorer');
const navCharts = document.getElementById('nav-charts');
const navBbsi = document.getElementById('nav-bbsi');
const navCustomerView = document.getElementById('nav-customer-view');
const navAbout = document.getElementById('nav-about');
const navRuntime = document.getElementById('nav-runtime');
const navModels = document.getElementById('nav-models');

const allNavItems = [navDashboard, navPnl, navBalanceSheet, navSavings, navLending,
    navCustomers, navPayments, navTreasuryCash, navTreasuryCapital, navTreasuryGilts,
    navSettings, navExplorer, navCharts, navBbsi, navCustomerView, navAbout, navRuntime, navModels];

let currentPage = 'dashboard';
let renderInterval = null;
let detailId = null; // for customer/payment/table detail pages
let detailTxPage = 1; // transaction page for customer detail, or explorer page
let detailAccountIdx = -1; // account index for customer-account page
let detailSort = ''; // explorer sort column
let detailDir = 'asc'; // explorer sort direction

// --- Page rendering ---

function renderPage() {
    switch (currentPage) {
        case 'dashboard':
            if (typeof goRender === 'function') outputDiv.innerHTML = goRender();
            break;
        case 'pnl':
            if (typeof goRenderPnL === 'function') outputDiv.innerHTML = goRenderPnL();
            break;
        case 'balance-sheet':
            if (typeof goRenderBalanceSheet === 'function') outputDiv.innerHTML = goRenderBalanceSheet();
            break;
        case 'savings':
            if (typeof goRenderProducts === 'function') outputDiv.innerHTML = goRenderProducts('savings');
            break;
        case 'lending':
            if (typeof goRenderProducts === 'function') outputDiv.innerHTML = goRenderProducts('lending');
            break;
        case 'customers':
            if (typeof goRenderCustomers === 'function') outputDiv.innerHTML = goRenderCustomers();
            break;
        case 'customer-detail':
            if (typeof goRenderCustomerDetail === 'function' && detailId)
                outputDiv.innerHTML = goRenderCustomerDetail(detailId, detailTxPage);
            break;
        case 'customer-account':
            if (typeof goRenderCustomerAccount === 'function' && detailId)
                outputDiv.innerHTML = goRenderCustomerAccount(detailId, detailAccountIdx, detailTxPage);
            break;
        case 'payments':
            if (typeof goRenderPayments === 'function') outputDiv.innerHTML = goRenderPayments();
            break;
        case 'payment-detail':
            if (typeof goRenderPaymentDetail === 'function' && detailId)
                outputDiv.innerHTML = goRenderPaymentDetail(detailId);
            break;
        case 'treasury-cash':
            if (typeof goRenderTreasuryCash === 'function') outputDiv.innerHTML = goRenderTreasuryCash();
            break;
        case 'treasury-capital':
            if (typeof goRenderTreasuryCapital === 'function') outputDiv.innerHTML = goRenderTreasuryCapital();
            break;
        case 'treasury-gilts':
            if (typeof goRenderTreasuryGilts === 'function') outputDiv.innerHTML = goRenderTreasuryGilts();
            break;
        case 'settings':
            if (typeof goRenderSettings === 'function') outputDiv.innerHTML = goRenderSettings();
            break;
        case 'explorer':
            if (typeof goRenderExplorer === 'function') outputDiv.innerHTML = goRenderExplorer();
            break;
        case 'explorer-table':
            if (typeof goRenderExplorerTable === 'function' && detailId)
                outputDiv.innerHTML = goRenderExplorerTable(detailId, detailTxPage, detailSort, detailDir);
            break;
        case 'charts':
            if (typeof goRenderCharts === 'function') outputDiv.innerHTML = goRenderCharts();
            break;
        case 'bbsi':
            if (typeof goRenderBBSI === 'function') outputDiv.innerHTML = goRenderBBSI();
            break;
        case 'customer-view':
            if (typeof goRenderCustomerViewReport === 'function' && detailId)
                outputDiv.innerHTML = goRenderCustomerViewReport(detailId);
            else if (detailId === null)
                outputDiv.innerHTML = '<div class="notification is-info is-light">Select a customer from the Customers page first.</div>';
            break;
        case 'about':
            if (typeof goRenderAbout === 'function') outputDiv.innerHTML = goRenderAbout();
            break;
        case 'runtime':
            if (typeof goRenderRuntime === 'function') outputDiv.innerHTML = goRenderRuntime();
            break;
        case 'models':
            if (typeof goRenderModels === 'function') {
                outputDiv.innerHTML = goRenderModels();
            }
            break;
    }
    updateStatus();
    attachDetailLinks();
    attachSettingsForm();
}

function updateStatus() {
    const bankRunning = typeof goIsRunning === 'function' && goIsRunning();
    const paymentsRunning = typeof goIsPaymentsRunning === 'function' && goIsPaymentsRunning();
    const addingCustomers = typeof goIsAddingCustomers === 'function' && goIsAddingCustomers();
    const running = bankRunning || paymentsRunning || addingCustomers;

    statusTag.textContent = running ? 'Running' : 'Stopped';
    statusTag.className = running ? 'tag is-warning' : 'tag is-success';

    startStopBtn.textContent = bankRunning ? 'Stop' : 'Run';
    startStopBtn.className = bankRunning ? 'button is-danger' : 'button is-success';

    autoPaymentsBtn.textContent = paymentsRunning ? 'Stop Auto' : 'Auto Send';
    autoPaymentsBtn.className = paymentsRunning ? 'button is-danger' : 'button is-success';
}

// Attach click handlers to dynamically rendered detail links (View buttons)
function attachDetailLinks() {
    // Customer account links (/customers/{id}/account/{idx})
    outputDiv.querySelectorAll('a[href^="/customers/"]').forEach(function(a) {
        var href = a.getAttribute('href');
        var accountMatch = href.match(/^\/customers\/([^/]+)\/account\/(\d+)/);
        if (accountMatch) {
            a.addEventListener('click', function(e) {
                e.preventDefault();
                var params = new URLSearchParams(href.split('?')[1] || '');
                detailId = accountMatch[1];
                detailAccountIdx = parseInt(accountMatch[2], 10);
                detailTxPage = parseInt(params.get('txpage')) || 1;
                showPage('customer-account');
            });
        } else {
            // Customer detail links (including txpage pagination)
            a.addEventListener('click', function(e) {
                e.preventDefault();
                var path = href.split('?')[0];
                var id = path.replace('/customers/', '');
                var params = new URLSearchParams(href.split('?')[1] || '');
                detailId = id;
                detailTxPage = parseInt(params.get('txpage')) || 1;
                showPage('customer-detail');
            });
        }
    });
    // Payment detail links
    outputDiv.querySelectorAll('a[href^="/payments/"]').forEach(function(a) {
        a.addEventListener('click', function(e) {
            e.preventDefault();
            var id = parseInt(a.getAttribute('href').replace('/payments/', ''), 10);
            detailId = id;
            showPage('payment-detail');
        });
    });
    // Customer view report links (if any)
    outputDiv.querySelectorAll('a[href^="/reports/customer-view"]').forEach(function(a) {
        a.addEventListener('click', function(e) {
            e.preventDefault();
            var url = new URL(a.href, window.location.origin);
            detailId = url.searchParams.get('id');
            showPage('customer-view');
        });
    });
    // Explorer table detail links
    outputDiv.querySelectorAll('a[href^="/internal/explorer/"]').forEach(function(a) {
        a.addEventListener('click', function(e) {
            e.preventDefault();
            var href = a.getAttribute('href');
            var path = href.split('?')[0];
            var name = path.replace('/internal/explorer/', '');
            var params = new URLSearchParams(href.split('?')[1] || '');
            detailId = name;
            detailTxPage = parseInt(params.get('page')) || 1;
            detailSort = params.get('sort') || '';
            detailDir = params.get('dir') || 'asc';
            showPage('explorer-table');
        });
    });
    // Explorer back link
    outputDiv.querySelectorAll('a[href="/internal/explorer"]').forEach(function(a) {
        a.addEventListener('click', function(e) {
            e.preventDefault();
            showPage('explorer');
        });
    });
    // PII auth links
    outputDiv.querySelectorAll('form[action="/auth/authorize"]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
            e.preventDefault();
            if (typeof goAuthorizePII === 'function') goAuthorizePII();
            renderPage();
        });
    });
    outputDiv.querySelectorAll('form[action="/auth/revoke"]').forEach(function(form) {
        form.addEventListener('submit', function(e) {
            e.preventDefault();
            if (typeof goRevokePII === 'function') goRevokePII();
            renderPage();
        });
    });
}

// Attach handler to settings form rendered by Go
function attachSettingsForm() {
    var form = outputDiv.querySelector('form[action="/settings"]');
    if (form) {
        form.addEventListener('submit', function(e) {
            e.preventDefault();
            var maxCust = parseInt(form.querySelector('[name="max_customers"]').value, 10);
            if (typeof goUpdateSettings === 'function') goUpdateSettings(maxCust);
            renderPage();
        });
    }
}

// --- Page navigation ---

function showPage(page) {
    if (renderInterval) {
        clearInterval(renderInterval);
        renderInterval = null;
    }

    currentPage = page;

    // Clear detail ID for non-detail pages
    if (page !== 'customer-detail' && page !== 'customer-account' && page !== 'payment-detail'
        && page !== 'customer-view' && page !== 'explorer-table') {
        detailId = null;
        detailTxPage = 1;
        detailAccountIdx = -1;
        detailSort = '';
        detailDir = 'asc';
    }

    // Update active nav
    allNavItems.forEach(function(item) { if (item) item.classList.remove('is-active'); });
    switch (page) {
        case 'dashboard': navDashboard.classList.add('is-active'); break;
        case 'pnl': navPnl.classList.add('is-active'); break;
        case 'balance-sheet': navBalanceSheet.classList.add('is-active'); break;
        case 'savings': navSavings.classList.add('is-active'); break;
        case 'lending': navLending.classList.add('is-active'); break;
        case 'customers': case 'customer-detail': case 'customer-account': navCustomers.classList.add('is-active'); break;
        case 'payments': case 'payment-detail': navPayments.classList.add('is-active'); break;
        case 'treasury-cash': navTreasuryCash.classList.add('is-active'); break;
        case 'treasury-capital': navTreasuryCapital.classList.add('is-active'); break;
        case 'treasury-gilts': navTreasuryGilts.classList.add('is-active'); break;
        case 'settings': navSettings.classList.add('is-active'); break;
        case 'explorer': case 'explorer-table': navExplorer.classList.add('is-active'); break;
        case 'charts': navCharts.classList.add('is-active'); break;
        case 'bbsi': navBbsi.classList.add('is-active'); break;
        case 'customer-view': navCustomerView.classList.add('is-active'); break;
        case 'about': navAbout.classList.add('is-active'); break;
        case 'runtime': navRuntime.classList.add('is-active'); break;
        case 'models': navModels.classList.add('is-active'); break;
    }

    // Show/hide controls
    controlsBank.style.display = page === 'dashboard' ? '' : 'none';
    controlsPayments.style.display = page === 'payments' ? '' : 'none';

    renderPage();

    // Auto-refresh for running sims
    const bankRunning = typeof goIsRunning === 'function' && goIsRunning();
    const paymentsRunning = typeof goIsPaymentsRunning === 'function' && goIsPaymentsRunning();
    const addingCustomers = typeof goIsAddingCustomers === 'function' && goIsAddingCustomers();
    if ((page === 'dashboard' && (bankRunning || addingCustomers)) || (page === 'payments' && paymentsRunning)) {
        renderInterval = setInterval(renderPage, 200);
    }
}

// --- Bank actions ---

function toggleStartStop() {
    if (goIsRunning()) {
        goStop();
        if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    } else {
        goStart();
        renderInterval = setInterval(renderPage, 200);
    }
    renderPage();
}

// --- Payments actions ---

function toggleAutoPayments() {
    if (goIsPaymentsRunning()) {
        goStopPayments();
        if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    } else {
        goStartPayments();
        renderInterval = setInterval(renderPage, 500);
    }
    renderPage();
}

// --- Init ---

window.wasmReady = function() {
    [startStopBtn, advanceBtn, addCustBtn, addCustN, resetBtn, exportBtn,
     sendPaymentBtn, autoPaymentsBtn, resetPaymentsBtn].forEach(function(btn) { btn.disabled = false; });
    showPage('dashboard');
};

async function loadWASM() {
    try {
        const go = new Go();
        const result = await WebAssembly.instantiateStreaming(
            fetch('main.wasm'), go.importObject
        );
        go.run(result.instance);
    } catch (err) {
        outputDiv.innerHTML = '<div class="notification is-danger">Failed to load WASM: ' + err.message + '</div>';
    }
}

// Navigation event listeners
navBrand.addEventListener('click', function(e) { e.preventDefault(); showPage('dashboard'); });
navDashboard.addEventListener('click', function(e) { e.preventDefault(); showPage('dashboard'); });
navPnl.addEventListener('click', function(e) { e.preventDefault(); showPage('pnl'); });
navBalanceSheet.addEventListener('click', function(e) { e.preventDefault(); showPage('balance-sheet'); });
navSavings.addEventListener('click', function(e) { e.preventDefault(); showPage('savings'); });
navLending.addEventListener('click', function(e) { e.preventDefault(); showPage('lending'); });
navCustomers.addEventListener('click', function(e) { e.preventDefault(); showPage('customers'); });
navPayments.addEventListener('click', function(e) { e.preventDefault(); showPage('payments'); });
navTreasuryCash.addEventListener('click', function(e) { e.preventDefault(); showPage('treasury-cash'); });
navTreasuryCapital.addEventListener('click', function(e) { e.preventDefault(); showPage('treasury-capital'); });
navTreasuryGilts.addEventListener('click', function(e) { e.preventDefault(); showPage('treasury-gilts'); });
navSettings.addEventListener('click', function(e) { e.preventDefault(); showPage('settings'); });
navExplorer.addEventListener('click', function(e) { e.preventDefault(); showPage('explorer'); });
navCharts.addEventListener('click', function(e) { e.preventDefault(); showPage('charts'); });
navBbsi.addEventListener('click', function(e) { e.preventDefault(); showPage('bbsi'); });
navCustomerView.addEventListener('click', function(e) { e.preventDefault(); showPage('customer-view'); });
navAbout.addEventListener('click', function(e) { e.preventDefault(); showPage('about'); });
navRuntime.addEventListener('click', function(e) { e.preventDefault(); showPage('runtime'); });
navModels.addEventListener('click', function(e) { e.preventDefault(); showPage('models'); });

// Bank controls
startStopBtn.addEventListener('click', toggleStartStop);
advanceBtn.addEventListener('click', function() { goAdvanceDay(); renderPage(); });
addCustBtn.addEventListener('click', function() {
    goAddCustomers(parseInt(addCustN.value, 10) || 100);
    renderPage();
    if (!renderInterval) {
        renderInterval = setInterval(renderPage, 200);
    }
});
resetBtn.addEventListener('click', function() {
    goReset();
    if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    renderPage();
});
exportBtn.addEventListener('click', function() {
    var data = goExport();
    if (data.startsWith('error:')) { alert(data); return; }
    var blob = new Blob([data], { type: 'application/octet-stream' });
    var a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'gobank.goluca';
    a.click();
    URL.revokeObjectURL(a.href);
});
importFile.addEventListener('change', function() {
    var file = importFile.files[0];
    if (!file) return;
    var reader = new FileReader();
    reader.onload = function() {
        var result = goImport(reader.result);
        if (result && result.startsWith('error:')) { alert(result); }
        else { renderPage(); }
    };
    reader.readAsText(file);
    importFile.value = '';
});

// Payment controls
sendPaymentBtn.addEventListener('click', function() { goSendPayment(); renderPage(); });
autoPaymentsBtn.addEventListener('click', toggleAutoPayments);
resetPaymentsBtn.addEventListener('click', function() {
    goResetPayments();
    if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    renderPage();
});

// Navbar burger (mobile)
document.addEventListener('DOMContentLoaded', function() {
    const burger = document.querySelector('.navbar-burger');
    if (burger) {
        burger.addEventListener('click', function() {
            burger.classList.toggle('is-active');
            document.getElementById(burger.dataset.target).classList.toggle('is-active');
        });
    }
});

loadWASM();
