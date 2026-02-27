const outputDiv = document.getElementById('output');
const statusTag = document.getElementById('status-tag');

// Bank controls
const controlsBank = document.getElementById('controls-bank');
const startStopBtn = document.getElementById('startStopBtn');
const advanceBtn = document.getElementById('advanceBtn');
const depositBtn = document.getElementById('depositBtn');
const withdrawBtn = document.getElementById('withdrawBtn');
const resetBtn = document.getElementById('resetBtn');

// Payment controls
const controlsPayments = document.getElementById('controls-payments');
const sendPaymentBtn = document.getElementById('sendPaymentBtn');
const autoPaymentsBtn = document.getElementById('autoPaymentsBtn');
const resetPaymentsBtn = document.getElementById('resetPaymentsBtn');

// Navigation
const navBank = document.getElementById('nav-bank');
const navPayments = document.getElementById('nav-payments');
const navModels = document.getElementById('nav-models');
const navBrand = document.getElementById('nav-brand');
const navItems = [navBank, navPayments, navModels];

let currentPage = 'bank';
let renderInterval = null;

// --- Page rendering ---

function renderBank() {
    if (typeof goRender === 'function') {
        outputDiv.innerHTML = goRender();
    }
}

function renderPayments() {
    if (typeof goRenderPayments === 'function') {
        outputDiv.innerHTML = goRenderPayments();
    }
}

function renderModels() {
    if (typeof goRenderModels === 'function') {
        outputDiv.innerHTML = goRenderModels();
    }
}

function render() {
    switch (currentPage) {
        case 'bank': renderBank(); break;
        case 'payments': renderPayments(); break;
        case 'models': renderModels(); break;
    }
    updateStatus();
}

function updateStatus() {
    const bankRunning = typeof goIsRunning === 'function' && goIsRunning();
    const paymentsRunning = typeof goIsPaymentsRunning === 'function' && goIsPaymentsRunning();
    const running = bankRunning || paymentsRunning;

    statusTag.textContent = running ? 'Running' : 'Stopped';
    statusTag.className = running ? 'tag is-warning' : 'tag is-success';

    // Bank button state
    startStopBtn.textContent = bankRunning ? 'Stop' : 'Run';
    startStopBtn.className = bankRunning ? 'button is-danger' : 'button is-success';

    // Payments button state
    autoPaymentsBtn.textContent = paymentsRunning ? 'Stop Auto' : 'Auto Send';
    autoPaymentsBtn.className = paymentsRunning ? 'button is-danger' : 'button is-success';
}

// --- Page navigation ---

function showPage(page) {
    // Stop any running interval
    if (renderInterval) {
        clearInterval(renderInterval);
        renderInterval = null;
    }

    currentPage = page;

    // Update active nav
    navItems.forEach(item => item.classList.remove('is-active'));
    switch (page) {
        case 'bank': navBank.classList.add('is-active'); break;
        case 'payments': navPayments.classList.add('is-active'); break;
        case 'models': navModels.classList.add('is-active'); break;
    }

    // Show/hide controls
    controlsBank.style.display = page === 'bank' ? '' : 'none';
    controlsPayments.style.display = page === 'payments' ? '' : 'none';

    render();

    // Restart interval if something is running
    const bankRunning = typeof goIsRunning === 'function' && goIsRunning();
    const paymentsRunning = typeof goIsPaymentsRunning === 'function' && goIsPaymentsRunning();
    if ((page === 'bank' && bankRunning) || (page === 'payments' && paymentsRunning)) {
        renderInterval = setInterval(render, 200);
    }
}

// --- Bank actions ---

function toggleStartStop() {
    if (goIsRunning()) {
        goStop();
        if (renderInterval) {
            clearInterval(renderInterval);
            renderInterval = null;
        }
    } else {
        goStart();
        renderInterval = setInterval(render, 200);
    }
    render();
}

// --- Payments actions ---

function toggleAutoPayments() {
    if (goIsPaymentsRunning()) {
        goStopPayments();
        if (renderInterval) {
            clearInterval(renderInterval);
            renderInterval = null;
        }
    } else {
        goStartPayments();
        renderInterval = setInterval(render, 500);
    }
    render();
}

// --- Init ---

window.wasmReady = function() {
    // Enable all buttons
    [startStopBtn, advanceBtn, depositBtn, withdrawBtn, resetBtn,
     sendPaymentBtn, autoPaymentsBtn, resetPaymentsBtn].forEach(btn => btn.disabled = false);

    showPage('bank');
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
navBrand.addEventListener('click', function(e) { e.preventDefault(); showPage('bank'); });
navBank.addEventListener('click', function(e) { e.preventDefault(); showPage('bank'); });
navPayments.addEventListener('click', function(e) { e.preventDefault(); showPage('payments'); });
navModels.addEventListener('click', function(e) { e.preventDefault(); showPage('models'); });

// Bank controls
startStopBtn.addEventListener('click', toggleStartStop);
advanceBtn.addEventListener('click', function() { goAdvanceDay(); render(); });
depositBtn.addEventListener('click', function() { goDeposit(); render(); });
withdrawBtn.addEventListener('click', function() { goWithdraw(); render(); });
resetBtn.addEventListener('click', function() {
    goReset();
    if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    render();
});

// Payment controls
sendPaymentBtn.addEventListener('click', function() { goSendPayment(); render(); });
autoPaymentsBtn.addEventListener('click', toggleAutoPayments);
resetPaymentsBtn.addEventListener('click', function() {
    goResetPayments();
    if (renderInterval) { clearInterval(renderInterval); renderInterval = null; }
    render();
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
