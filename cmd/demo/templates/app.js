const outputDiv = document.getElementById('output');
const statusTag = document.getElementById('status-tag');
const startStopBtn = document.getElementById('startStopBtn');
const advanceBtn = document.getElementById('advanceBtn');
const depositBtn = document.getElementById('depositBtn');
const withdrawBtn = document.getElementById('withdrawBtn');
const resetBtn = document.getElementById('resetBtn');

let renderInterval = null;

function render() {
    if (typeof goRender === 'function') {
        outputDiv.innerHTML = goRender();
        updateStatus();
    }
}

function updateStatus() {
    const running = typeof goIsRunning === 'function' && goIsRunning();
    statusTag.textContent = running ? 'Running' : 'Stopped';
    statusTag.className = running ? 'tag is-warning' : 'tag is-success';
    startStopBtn.textContent = running ? 'Stop' : 'Run';
    startStopBtn.className = running ? 'button is-danger' : 'button is-success';
}

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

window.wasmReady = function() {
    startStopBtn.disabled = false;
    advanceBtn.disabled = false;
    depositBtn.disabled = false;
    withdrawBtn.disabled = false;
    resetBtn.disabled = false;
    render();
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

startStopBtn.addEventListener('click', toggleStartStop);
advanceBtn.addEventListener('click', function() { goAdvanceDay(); render(); });
depositBtn.addEventListener('click', function() { goDeposit(); render(); });
withdrawBtn.addEventListener('click', function() { goWithdraw(); render(); });
resetBtn.addEventListener('click', function() {
    goReset();
    if (renderInterval) {
        clearInterval(renderInterval);
        renderInterval = null;
    }
    render();
});

loadWASM();
