// Bientot Dashboard - Main Application

const API_BASE = '/api';

// State
let charts = {};
let refreshInterval = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    initDashboard();
    startAutoRefresh();
});

async function initDashboard() {
    await loadOverview();
    await loadCharts();
}

function startAutoRefresh() {
    refreshInterval = setInterval(loadOverview, 30000); // 30 seconds
}

// API Functions
async function fetchJSON(endpoint) {
    const response = await fetch(`${API_BASE}${endpoint}`);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return response.json();
}

// Overview
async function loadOverview() {
    try {
        const data = await fetchJSON('/overview');
        updateGlobalStatus(data);
        updateOverviewCards(data);
        updateSystemMetrics(data);
        renderAlerts(data.alerts || []);
        updateLastUpdate();
    } catch (error) {
        console.error('Failed to load overview:', error);
        updateGlobalStatus({ health: 'unknown' });
    }
}

function updateLastUpdate() {
    const el = document.getElementById('last-update');
    el.textContent = `Updated: ${new Date().toLocaleTimeString()}`;
}

function updateGlobalStatus(data) {
    const container = document.getElementById('global-status');
    const health = data.health || 'unknown';
    const healthClass = `bg-status-${health}`;
    const healthText = health.charAt(0).toUpperCase() + health.slice(1);
    const alertCount = (data.alerts || []).length;

    container.innerHTML = `
        <span class="w-3 h-3 rounded-full ${healthClass}"></span>
        <span class="text-sm">${healthText}</span>
        ${alertCount > 0 ? `<span class="text-xs bg-red-600 px-2 py-0.5 rounded-full">${alertCount}</span>` : ''}
    `;
}

function updateOverviewCards(data) {
    const container = document.getElementById('overview-cards');
    const cards = [];

    // Uptime
    if (data.uptime) {
        cards.push({
            label: 'Uptime',
            value: formatDurationSec(data.uptime),
            color: 'text-blue-400'
        });
    }

    // Containers
    if (data.containers_total) {
        const running = data.container_running?.value || 0;
        const total = data.containers_total.value;
        cards.push({
            label: 'Containers',
            value: `${running}/${total}`,
            color: running === total ? 'text-green-400' : 'text-yellow-400'
        });
    }

    // ZFS
    if (data.zfs_pool_health) {
        const health = data.zfs_pool_health.value;
        const healthText = health === 2 ? 'Healthy' : health === 1 ? 'Degraded' : 'Error';
        cards.push({
            label: 'ZFS',
            value: healthText,
            color: health === 2 ? 'text-green-400' : 'text-red-400'
        });
    }

    // CrowdSec bans
    if (data.crowdsec_bans_active) {
        cards.push({
            label: 'Active Bans',
            value: data.crowdsec_bans_active.value,
            color: 'text-orange-400'
        });
    }

    // Alerts
    cards.push({
        label: 'Alerts',
        value: (data.alerts || []).length,
        color: (data.alerts || []).length > 0 ? 'text-red-400' : 'text-green-400'
    });

    // Health
    cards.push({
        label: 'Health',
        value: (data.health || 'unknown').toUpperCase(),
        color: `status-${data.health || 'unknown'}`
    });

    container.innerHTML = cards.map(card => `
        <div class="bg-gray-800 rounded-lg p-4 text-center">
            <div class="text-xs text-gray-500 uppercase tracking-wide">${card.label}</div>
            <div class="text-xl font-bold ${card.color}">${card.value}</div>
        </div>
    `).join('');
}

function updateSystemMetrics(data) {
    // Memory
    if (data.node_memory_MemAvailable_bytes && data.node_memory_MemTotal_bytes) {
        const avail = data.node_memory_MemAvailable_bytes.value;
        const total = data.node_memory_MemTotal_bytes.value;
        const usedPercent = ((total - avail) / total * 100).toFixed(1);

        document.getElementById('memory-value').textContent = `${usedPercent}%`;
        document.getElementById('memory-bar').style.width = `${usedPercent}%`;
        updateBarColor('memory-bar', parseFloat(usedPercent));
    }

    // Disk
    if (data.node_filesystem_avail_bytes && data.node_filesystem_size_bytes) {
        const avail = data.node_filesystem_avail_bytes.value;
        const size = data.node_filesystem_size_bytes.value;
        const usedPercent = ((size - avail) / size * 100).toFixed(1);

        document.getElementById('disk-value').textContent = `${usedPercent}%`;
        document.getElementById('disk-bar').style.width = `${usedPercent}%`;
        updateBarColor('disk-bar', parseFloat(usedPercent));
    }

    // ZFS disk if available
    if (data.zfs_pool_used_percent) {
        const usedPercent = data.zfs_pool_used_percent.value.toFixed(1);
        document.getElementById('disk-value').textContent = `${usedPercent}%`;
        document.getElementById('disk-bar').style.width = `${usedPercent}%`;
        updateBarColor('disk-bar', parseFloat(usedPercent));
    }
}

function updateBarColor(id, percent) {
    const el = document.getElementById(id);
    el.classList.remove('bg-green-500', 'bg-yellow-500', 'bg-red-500', 'bg-blue-500');
    if (percent >= 90) {
        el.classList.add('bg-red-500');
    } else if (percent >= 75) {
        el.classList.add('bg-yellow-500');
    } else {
        el.classList.add('bg-green-500');
    }
}

// Alerts
function renderAlerts(alerts) {
    const container = document.getElementById('alerts-container');
    const countEl = document.getElementById('alerts-count');

    countEl.textContent = alerts.length;
    countEl.className = alerts.length > 0
        ? 'text-xs bg-red-600 px-2 py-0.5 rounded-full'
        : 'text-xs bg-gray-700 px-2 py-0.5 rounded-full';

    if (alerts.length === 0) {
        container.innerHTML = `
            <div class="text-gray-500 text-sm py-4 bg-gray-800 rounded-lg text-center">
                No active alerts
            </div>
        `;
        return;
    }

    container.innerHTML = alerts.map(alert => `
        <div class="bg-gray-800 border-l-4 ${alert.severity === 'critical' ? 'border-red-500' : 'border-yellow-500'} rounded-r-lg p-4 flex justify-between items-center">
            <div>
                <div class="font-medium ${alert.severity === 'critical' ? 'text-red-400' : 'text-yellow-400'}">
                    ${alert.name}
                </div>
                <div class="text-sm text-gray-400">${alert.message}</div>
                <div class="text-xs text-gray-500 mt-1">
                    Fired: ${formatTime(alert.fired_at)}
                </div>
            </div>
            <button
                onclick="acknowledgeAlert('${alert.id}')"
                class="px-3 py-1 text-sm bg-gray-700 hover:bg-gray-600 rounded ${alert.acknowledged ? 'opacity-50' : ''}"
                ${alert.acknowledged ? 'disabled' : ''}
            >
                ${alert.acknowledged ? 'Ack' : 'Acknowledge'}
            </button>
        </div>
    `).join('');
}

async function acknowledgeAlert(id) {
    try {
        await fetch(`${API_BASE}/alerts/${id}/ack`, { method: 'POST' });
        loadOverview();
    } catch (error) {
        console.error('Failed to acknowledge alert:', error);
    }
}

// Charts
async function loadCharts() {
    const metrics = [
        { id: 'chart-cpu', name: 'node_cpu_seconds_total' },
        { id: 'chart-memory', name: 'node_memory_MemAvailable_bytes' }
    ];

    for (const m of metrics) {
        try {
            const to = new Date().toISOString();
            const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
            const data = await fetchJSON(`/metrics/${m.name}?from=${from}&to=${to}&resolution=5min`);

            if (data.points && data.points.length > 0) {
                renderChart(m.id, data.points);
            } else {
                showNoData(m.id);
            }
        } catch (error) {
            showNoData(m.id);
        }
    }
}

function showNoData(elementId) {
    const el = document.getElementById(elementId);
    if (el) {
        el.innerHTML = '<div class="text-gray-500 text-sm flex items-center justify-center h-full">No data available</div>';
    }
}

function renderChart(elementId, points) {
    const el = document.getElementById(elementId);
    if (!el || points.length === 0) return;

    el.innerHTML = '';

    const timestamps = points.map(p => new Date(p.timestamp).getTime() / 1000);
    const values = points.map(p => p.value);

    const opts = {
        width: el.clientWidth,
        height: 180,
        scales: {
            x: { time: true },
            y: { auto: true }
        },
        series: [
            {},
            {
                stroke: '#3b82f6',
                fill: 'rgba(59, 130, 246, 0.1)',
                width: 2
            }
        ],
        axes: [
            { stroke: '#6b7280', grid: { stroke: '#374151' }, ticks: { stroke: '#374151' } },
            { stroke: '#6b7280', grid: { stroke: '#374151' }, ticks: { stroke: '#374151' } }
        ]
    };

    const chart = new uPlot(opts, [timestamps, values], el);
    charts[elementId] = chart;

    const resizeObserver = new ResizeObserver(() => {
        chart.setSize({ width: el.clientWidth, height: 180 });
    });
    resizeObserver.observe(el);
}

// Utilities
function formatDurationSec(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);

    if (hours >= 24) {
        const days = Math.floor(hours / 24);
        return `${days}d ${hours % 24}h`;
    }
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
}

function formatTime(isoString) {
    if (!isoString) return '';
    const date = new Date(isoString);
    return date.toLocaleString();
}
