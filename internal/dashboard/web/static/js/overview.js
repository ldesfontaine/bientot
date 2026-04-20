// overview.js — manages the two charts on the Overview page.
// Loaded as a non-module script; uPlot is available as window.uPlot.
// Guards against running on pages without the chart containers.

(function () {
  'use strict';

  // ─── Configuration ─────────────────────────────────────

  const machineID = window.location.pathname.split('/')[2];
  let currentRange = '24h';

  const charts = {
    load: null,
    memory: null,
  };

  // ─── Formatters ────────────────────────────────────────

  function fmtBytesShort(bytes) {
    if (bytes === null || bytes === undefined) return '—';
    const b = Math.abs(bytes);
    if (b >= 1e12) return (bytes / 1e12).toFixed(2) + ' TB';
    if (b >= 1e9)  return (bytes / 1e9).toFixed(1)  + ' GB';
    if (b >= 1e6)  return (bytes / 1e6).toFixed(1)  + ' MB';
    if (b >= 1e3)  return (bytes / 1e3).toFixed(0)  + ' KB';
    return bytes + ' B';
  }

  function fmtLoad(v) {
    return v === null ? '—' : v.toFixed(2);
  }

  // fmtXAxis renders an X-axis tick (seconds since epoch) as local time.
  // Adapts format based on currentRange:
  //   ≤ 24h: "HH:MM"
  //   >  24h: "DD/MM HH:MM"
  function fmtXAxis(secondsEpoch) {
    const d = new Date(secondsEpoch * 1000);
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');

    const isLong = currentRange === '168h' || currentRange === '7d';

    if (isLong) {
      const dd = String(d.getDate()).padStart(2, '0');
      const mo = String(d.getMonth() + 1).padStart(2, '0');
      return `${dd}/${mo} ${hh}:${mm}`;
    }

    return `${hh}:${mm}`;
  }

  // ─── Fetch helpers ─────────────────────────────────────

  async function fetchPoints(metricName, range) {
    const url = `/api/agents/${machineID}/metric-points?name=${metricName}&range=${range}`;
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`fetch ${metricName} failed: HTTP ${res.status}`);
    }
    const data = await res.json();
    return data.points || [];
  }

  // uPlot expects [xAxis, ...series]; xAxis in seconds.
  // Aligns multiple series defensively on the union of timestamps.
  function pointsToUPlotData(pointSets) {
    const timestampSet = new Set();
    pointSets.forEach(pts => pts.forEach(p => timestampSet.add(p.t)));
    const timestamps = Array.from(timestampSet).sort((a, b) => a - b);

    const xAxis = timestamps.map(ms => ms / 1000);
    const series = pointSets.map(pts => {
      const byTs = new Map(pts.map(p => [p.t, p.v]));
      return timestamps.map(ts => {
        const v = byTs.get(ts);
        return v === undefined ? null : v;
      });
    });

    return [xAxis, ...series];
  }

  // ─── Chart options ─────────────────────────────────────

  function loadChartOpts(container) {
    return {
      title: '',
      width: container.clientWidth,
      height: 220,
      series: [
        {},
        { label: '1m',  stroke: 'oklch(72% 0.09 195)', width: 1.5 },
        { label: '5m',  stroke: 'oklch(72% 0.09 220)', width: 1.5 },
        { label: '15m', stroke: 'oklch(60% 0.05 240)', width: 1.5 },
      ],
      axes: [
        {
          stroke: '#6b727d',
          values: (_, splits) => splits.map(v => fmtXAxis(v)),
        },
        {
          stroke: '#6b727d',
          values: (_, splits) => splits.map(v => fmtLoad(v)),
        },
      ],
      scales: { x: { time: true } },
      legend: { show: true },
    };
  }

  function memoryChartOpts(container) {
    return {
      title: '',
      width: container.clientWidth,
      height: 220,
      series: [
        {},
        { label: 'Memory available', stroke: 'oklch(72% 0.15 150)', width: 1.5 },
      ],
      axes: [
        {
          stroke: '#6b727d',
          values: (_, splits) => splits.map(v => fmtXAxis(v)),
        },
        {
          stroke: '#6b727d',
          values: (_, splits) => splits.map(v => fmtBytesShort(v)),
        },
      ],
      scales: { x: { time: true } },
      legend: { show: true },
    };
  }

  // ─── Chart lifecycle ───────────────────────────────────

  async function renderLoadChart() {
    const container = document.getElementById('chart-load');
    const status = document.getElementById('chart-load-status');
    if (!container) return;

    status.textContent = 'loading…';
    status.className = 'chart-status';

    try {
      const [p1, p5, p15] = await Promise.all([
        fetchPoints('load_average_1m', currentRange),
        fetchPoints('load_average_5m', currentRange),
        fetchPoints('load_average_15m', currentRange),
      ]);

      const data = pointsToUPlotData([p1, p5, p15]);

      if (charts.load) {
        charts.load.destroy();
      }
      charts.load = new uPlot(loadChartOpts(container), data, container);
      status.textContent = '';
    } catch (err) {
      console.error('load chart error:', err);
      status.textContent = 'Failed to load data';
      status.className = 'chart-status chart-status-error';
    }
  }

  async function renderMemoryChart() {
    const container = document.getElementById('chart-memory');
    const status = document.getElementById('chart-memory-status');
    if (!container) return;

    status.textContent = 'loading…';
    status.className = 'chart-status';

    try {
      const pts = await fetchPoints('memory_available_bytes', currentRange);
      const data = pointsToUPlotData([pts]);

      if (charts.memory) {
        charts.memory.destroy();
      }
      charts.memory = new uPlot(memoryChartOpts(container), data, container);
      status.textContent = '';
    } catch (err) {
      console.error('memory chart error:', err);
      status.textContent = 'Failed to load data';
      status.className = 'chart-status chart-status-error';
    }
  }

  function renderAllCharts() {
    renderLoadChart();
    renderMemoryChart();
  }

  // ─── Range selector ────────────────────────────────────

  function wireRangeSelector() {
    const buttons = document.querySelectorAll('.range-btn');
    buttons.forEach(btn => {
      btn.addEventListener('click', () => {
        buttons.forEach(b => b.classList.remove('active'));
        btn.classList.add('active');

        currentRange = btn.dataset.range;
        renderAllCharts();
      });
    });
  }

  // ─── Resize handling ───────────────────────────────────

  function wireResize() {
    let timeout = null;
    window.addEventListener('resize', () => {
      clearTimeout(timeout);
      timeout = setTimeout(() => renderAllCharts(), 200);
    });
  }

  // ─── Init (guarded: only if chart containers exist) ────

  function init() {
    if (!document.getElementById('chart-load')) return;
    wireRangeSelector();
    wireResize();
    renderAllCharts();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
