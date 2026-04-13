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
    await loadLogs();
    await loadLogStats();
    await loadServices();
}

function startAutoRefresh() {
    refreshInterval = setInterval(() => {
        loadOverview();
        loadLogStats();
    }, 30000);
    // Refresh logs every 5 minutes (warm interval)
    setInterval(loadLogs, 300000);
    // Refresh services every 5 minutes (cold interval)
    setInterval(loadServices, 300000);
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

// Logs
async function loadLogs() {
    try {
        const source = document.getElementById('log-filter-source')?.value || '';
        const severity = document.getElementById('log-filter-severity')?.value || '';
        let endpoint = '/logs?limit=100';
        if (source) endpoint += `&source=${source}`;
        if (severity) endpoint += `&severity=${severity}`;

        const data = await fetchJSON(endpoint);
        renderLogs(data.logs || []);
    } catch (error) {
        console.error('Failed to load logs:', error);
    }
}

async function loadLogStats() {
    try {
        const stats = await fetchJSON('/logs/stats');
        updateLogStatsHeader(stats);
    } catch (error) {
        // Logs module may not be active
    }
}

function updateLogStatsHeader(stats) {
    const el = document.getElementById('log-stats');
    if (!el) return;

    const errCount = (stats.by_severity?.error || 0);
    const critCount = (stats.by_severity?.critical || 0);

    if (errCount === 0 && critCount === 0) {
        el.textContent = '';
        return;
    }

    const parts = [];
    if (errCount > 0) parts.push(`${errCount} err`);
    if (critCount > 0) parts.push(`${critCount} crit`);
    el.innerHTML = `<span class="bg-gray-800 px-2 py-0.5 rounded">LOGS ${parts.join(' \u2502 ')}</span>`;

    // Show logs section if we have data
    const section = document.getElementById('logs-section');
    if (section) section.classList.remove('hidden');
}

function renderLogs(logs) {
    const section = document.getElementById('logs-section');
    const tbody = document.getElementById('logs-table-body');
    const countEl = document.getElementById('logs-count');
    if (!tbody || !section) return;

    if (logs.length > 0) {
        section.classList.remove('hidden');
    }

    countEl.textContent = logs.length;

    const severityColors = {
        info: 'text-gray-400',
        warning: 'text-yellow-400',
        error: 'text-orange-400',
        critical: 'text-red-400'
    };

    const severityBg = {
        info: '',
        warning: '',
        error: 'bg-orange-900/20',
        critical: 'bg-red-900/20'
    };

    tbody.innerHTML = logs.map(log => {
        const color = severityColors[log.severity] || 'text-gray-400';
        const bg = severityBg[log.severity] || '';
        const ts = formatTime(log.timestamp);
        const msg = escapeHtml(log.message || '');
        const parsedInfo = log.parsed ? formatParsed(log.parsed) : '';

        return `
            <tr class="${bg} hover:bg-gray-700/50">
                <td class="px-3 py-1.5 text-xs text-gray-500 whitespace-nowrap">${ts}</td>
                <td class="px-3 py-1.5">
                    <span class="text-xs px-1.5 py-0.5 rounded bg-gray-700">${log.source}</span>
                </td>
                <td class="px-3 py-1.5 text-xs font-medium ${color}">${log.severity}</td>
                <td class="px-3 py-1.5 text-xs text-gray-300 truncate max-w-lg" title="${msg}">
                    ${msg}
                    ${parsedInfo ? `<span class="text-gray-500 ml-1">${parsedInfo}</span>` : ''}
                </td>
            </tr>
        `;
    }).join('');
}

function formatParsed(parsed) {
    const parts = [];
    if (parsed.src_ip) parts.push(parsed.src_ip);
    if (parsed.user) parts.push(`user:${parsed.user}`);
    if (parsed.container) parts.push(`container:${parsed.container}`);
    if (parsed.scenario) parts.push(parsed.scenario);
    return parts.length > 0 ? `[${parts.join(', ')}]` : '';
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Wire up log filters
document.addEventListener('DOMContentLoaded', () => {
    const sourceFilter = document.getElementById('log-filter-source');
    const sevFilter = document.getElementById('log-filter-severity');
    if (sourceFilter) sourceFilter.addEventListener('change', loadLogs);
    if (sevFilter) sevFilter.addEventListener('change', loadLogs);
});

// Services
const SERVICE_ICONS = {
    bell: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/></svg>`,
    cloud: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z"/></svg>`,
    lock: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>`,
    file: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/></svg>`,
    globe: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9"/></svg>`,
    image: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>`,
    shield: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>`,
    activity: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>`,
    search: `<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>`,
};

async function loadServices() {
    try {
        const data = await fetchJSON('/services');
        renderServices(data.services || []);
    } catch (error) {
        console.error('Failed to load services:', error);
    }
}

function renderServices(services) {
    const section = document.getElementById('services-section');
    const container = document.getElementById('services-container');
    if (!section || !container) return;

    if (services.length === 0) {
        section.classList.add('hidden');
        return;
    }

    section.classList.remove('hidden');

    // Sort: public first, then alphabetical
    services.sort((a, b) => {
        if (a.public !== b.public) return b.public - a.public;
        return a.name.localeCompare(b.name);
    });

    container.innerHTML = services.map(svc => {
        const icon = SERVICE_ICONS[svc.icon] || SERVICE_ICONS.globe;
        const isRunning = svc.status === 'running';
        const statusDot = isRunning ? 'bg-green-500' : 'bg-red-500';
        const healthBadge = svc.health === 'healthy'
            ? '<span class="text-xs bg-green-900/50 text-green-400 px-1.5 py-0.5 rounded">healthy</span>'
            : svc.health === 'unhealthy'
            ? '<span class="text-xs bg-red-900/50 text-red-400 px-1.5 py-0.5 rounded">unhealthy</span>'
            : '';
        const accessBadge = svc.public
            ? '<span class="text-xs bg-blue-900/50 text-blue-400 px-1.5 py-0.5 rounded">public</span>'
            : '<span class="text-xs bg-gray-700 text-gray-400 px-1.5 py-0.5 rounded">VPN</span>';
        const machineBadge = `<span class="text-xs text-gray-500">${svc.machine}</span>`;

        return `
            <a href="${escapeHtml(svc.url)}" target="_blank" rel="noopener noreferrer"
               class="bg-gray-800 rounded-lg p-4 hover:bg-gray-750 transition-colors group block">
                <div class="flex items-start justify-between mb-3">
                    <div class="text-gray-400 group-hover:text-blue-400 transition-colors">
                        ${icon}
                    </div>
                    <span class="w-2.5 h-2.5 rounded-full ${statusDot}"></span>
                </div>
                <div class="font-medium text-gray-100 mb-1">${escapeHtml(svc.name)}</div>
                <div class="text-xs text-gray-500 truncate mb-3">${escapeHtml(svc.url)}</div>
                <div class="flex items-center gap-2 flex-wrap">
                    ${accessBadge}
                    ${healthBadge}
                    ${machineBadge}
                </div>
            </a>
        `;
    }).join('');
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
