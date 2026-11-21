(() => {
  const $ = (id) => document.getElementById(id);

  const els = {
    connBadge: $('conn-badge'),
    runBadge: $('run-badge'),
    modeBadge: $('mode-badge'),
    alertBanner: $('alert-banner'),
    toggleRunBtn: $('toggle-run-btn'),
    appliedSpeed: $('applied-speed'),
    targetSpeed: $('target-speed'),
    fanGaugeFill: $('fan-gauge-fill'),
    modeAutoBtn: $('mode-auto-btn'),
    modeManualBtn: $('mode-manual-btn'),
    manualSlider: $('manual-slider'),
    manualSliderVal: $('manual-slider-val'),
    speedDownBtn: $('speed-down-btn'),
    speedUpBtn: $('speed-up-btn'),
    hrConnBadge: $('hr-conn-badge'),
    liveBpm: $('live-bpm'),
    effBpm: $('eff-bpm'),
    effLabel: $('eff-label'),
    hrDeviceName: $('hr-device-name'),
    fanDeviceName: $('fan-device-name'),
    lastHrAge: $('last-hr-age'),
    historyMeta: $('history-meta'),
    historySvg: $('history-svg'),
    curveSvg: $('curve-svg'),
    curveForm: $('curve-form'),
    saveStatus: $('save-status'),
    cfgMinHr: $('cfg-min-hr'),
    cfgMinSpeed: $('cfg-min-speed'),
    cfgMaxHr: $('cfg-max-hr'),
    cfgMaxSpeed: $('cfg-max-speed'),
    cfgWindow: $('cfg-window'),
    cfgPercentile: $('cfg-percentile'),
  };

  let latestState = null;
  let sliderDragging = false;
  let curveInitialized = false;

  async function postJSON(url, payload) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: payload !== undefined ? JSON.stringify(payload) : undefined,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    return data;
  }

  function setBadge(el, text, kind) {
    el.textContent = text;
    el.className = `badge badge-${kind}`;
  }

  function updateSliderProgress(val) {
    els.manualSlider.style.setProperty('--progress', `${val}%`);
  }

  function render(st) {
    latestState = st;

    // Top badges
    if (st.running) {
      setBadge(els.runBadge, 'RUNNING', 'ok');
      els.toggleRunBtn.textContent = 'Stop Fan';
      els.toggleRunBtn.className = 'btn btn-danger';
    } else {
      setBadge(els.runBadge, 'STOPPED', 'muted');
      els.toggleRunBtn.textContent = 'Start Fan';
      els.toggleRunBtn.className = 'btn btn-primary';
    }

    const isManual = st.mode === 'MANUAL';
    setBadge(els.modeBadge, st.mode || 'AUTO', isManual ? 'warn' : 'brand');
    els.modeAutoBtn.setAttribute('aria-pressed', String(!isManual));
    els.modeManualBtn.setAttribute('aria-pressed', String(isManual));

    // Fan metrics
    els.appliedSpeed.textContent = `${st.applied_speed}%`;
    els.targetSpeed.textContent = `${st.target_speed}%`;
    els.fanGaugeFill.style.width = `${Math.max(0, Math.min(100, st.applied_speed))}%`;

    if (!sliderDragging) {
      els.manualSlider.value = String(st.target_speed);
      els.manualSliderVal.textContent = `${st.target_speed}%`;
      updateSliderProgress(st.target_speed);
    }

    // HR metrics
    if (st.hr_connected) {
      setBadge(els.hrConnBadge, 'Connected', 'ok');
    } else {
      setBadge(els.hrConnBadge, 'Disconnected', 'muted');
    }

    els.liveBpm.textContent = st.bpm > 0 ? String(st.bpm) : '--';
    els.effBpm.textContent = st.effective_bpm > 0 ? String(st.effective_bpm) : '--';
    const pct = st.percentile || st.curve.percentile || 80;
    const winSec = st.window_seconds || st.curve.window_seconds || 120;
    els.effLabel.textContent = `P${pct} (${st.window_count || 0}/${winSec}s)`;

    if (st.hr_device) {
      els.hrDeviceName.textContent = st.hr_device.name || st.hr_device.id;
    } else {
      els.hrDeviceName.textContent = 'None selected';
    }

    if (st.fan_device) {
      const host = st.fan_device.host ? ` (${st.fan_device.host})` : '';
      els.fanDeviceName.textContent = `${st.fan_device.name || st.fan_device.id}${host}`;
    } else {
      els.fanDeviceName.textContent = 'None selected';
    }

    if (st.last_hr_at) {
      const ageSec = Math.max(0, Math.round((Date.now() - Date.parse(st.last_hr_at)) / 1000));
      els.lastHrAge.textContent = `${ageSec}s ago`;
    } else {
      els.lastHrAge.textContent = '—';
    }

    const errText = st.last_err || st.err_msg || '';
    if (errText) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = errText;
    } else {
      els.alertBanner.hidden = true;
    }

    // Sync curve inputs only if not currently editing any field
    const formFocused = els.curveForm.contains(document.activeElement);
    if (!curveInitialized || !formFocused) {
      els.cfgMinHr.value = st.curve.min_hr;
      els.cfgMinSpeed.value = st.curve.min_speed;
      els.cfgMaxHr.value = st.curve.max_hr;
      els.cfgMaxSpeed.value = st.curve.max_speed;
      els.cfgWindow.value = st.curve.window_seconds;
      els.cfgPercentile.value = st.curve.percentile;
      curveInitialized = true;
    }

    els.historyMeta.textContent = `${winSec}s window`;
    renderHistoryChart(st.history || [], st.effective_bpm, pct);
    renderCurveChart(st.curve, st.bpm, st.effective_bpm);
  }

  function renderHistoryChart(history, effBpm, pct) {
    const svg = els.historySvg;
    const W = 480;
    const H = 180;
    const pad = { l: 38, r: 12, t: 14, b: 24 };
    const pw = W - pad.l - pad.r;
    const ph = H - pad.t - pad.b;

    const valid = history.filter((v) => v > 0);
    if (valid.length === 0) {
      svg.innerHTML = `<text x="${W / 2}" y="${H / 2}" text-anchor="middle" fill="currentColor" opacity="0.5" font-size="13">Waiting for heart rate samples…</text>`;
      return;
    }

    let minY = Math.min(...valid, effBpm || 999) - 8;
    let maxY = Math.max(...valid, effBpm || 0) + 8;
    if (minY < 40) minY = 40;
    if (maxY - minY < 20) maxY = minY + 20;

    const xOf = (i) => pad.l + (history.length <= 1 ? pw : (i / (history.length - 1)) * pw);
    const yOf = (bpm) => pad.t + ph - ((bpm - minY) / (maxY - minY)) * ph;

    // Build contiguous segments for non-zero readings
    let segments = [];
    let cur = [];
    for (let i = 0; i < history.length; i++) {
      if (history[i] > 0) {
        cur.push([xOf(i), yOf(history[i])]);
      } else if (cur.length > 0) {
        segments.push(cur);
        cur = [];
      }
    }
    if (cur.length > 0) segments.push(cur);

    const gridLines = [minY, Math.round((minY + maxY) / 2), maxY].map((val) => {
      const y = yOf(val).toFixed(1);
      return `<line x1="${pad.l}" y1="${y}" x2="${W - pad.r}" y2="${y}" stroke="var(--border)" stroke-dasharray="3 3" />
              <text x="${pad.l - 6}" y="${Number(y) + 4}" text-anchor="end" fill="var(--muted)" font-size="11">${val}</text>`;
    }).join('');

    const paths = segments.map((pts) => {
      const d = pts.map((p, idx) => `${idx === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
      return `<path d="${d}" fill="none" stroke="var(--brand)" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" />`;
    }).join('');

    let effLine = '';
    if (effBpm > 0) {
      const ey = yOf(effBpm).toFixed(1);
      effLine = `<line x1="${pad.l}" y1="${ey}" x2="${W - pad.r}" y2="${ey}" stroke="var(--warn)" stroke-width="1.5" stroke-dasharray="5 4" />
                 <text x="${W - pad.r - 4}" y="${Number(ey) - 4}" text-anchor="end" fill="var(--warn)" font-size="11">P${pct}: ${effBpm}</text>`;
    }

    svg.innerHTML = `${gridLines}${paths}${effLine}
      <text x="${pad.l}" y="${H - 5}" fill="var(--muted)" font-size="11">-${history.length}s</text>
      <text x="${W - pad.r}" y="${H - 5}" text-anchor="end" fill="var(--muted)" font-size="11">now</text>`;
  }

  function mapSpeed(curve, bpm) {
    if (bpm <= 0) return 0;
    if (bpm <= curve.min_hr) return curve.min_speed;
    if (bpm >= curve.max_hr) return curve.max_speed;
    const ratio = (bpm - curve.min_hr) / (curve.max_hr - curve.min_hr);
    return Math.round(curve.min_speed + ratio * (curve.max_speed - curve.min_speed));
  }

  function renderCurveChart(curve, liveBpm, effBpm) {
    const svg = els.curveSvg;
    const W = 480;
    const H = 180;
    const pad = { l: 38, r: 16, t: 14, b: 26 };
    const pw = W - pad.l - pad.r;
    const ph = H - pad.t - pad.b;

    const minX = Math.max(40, curve.min_hr - 20);
    const maxX = Math.min(230, curve.max_hr + 20);
    const xOf = (bpm) => {
      const clamped = Math.max(minX, Math.min(maxX, bpm));
      return pad.l + ((clamped - minX) / (maxX - minX)) * pw;
    };
    const yOf = (spd) => pad.t + ph - (spd / 100) * ph;

    const gridY = [0, 50, 100].map((spd) => {
      const y = yOf(spd).toFixed(1);
      return `<line x1="${pad.l}" y1="${y}" x2="${W - pad.r}" y2="${y}" stroke="var(--border)" stroke-dasharray="3 3" />
              <text x="${pad.l - 6}" y="${Number(y) + 4}" text-anchor="end" fill="var(--muted)" font-size="11">${spd}%</text>`;
    }).join('');

    const pts = [
      [xOf(minX), yOf(curve.min_speed)],
      [xOf(curve.min_hr), yOf(curve.min_speed)],
      [xOf(curve.max_hr), yOf(curve.max_speed)],
      [xOf(maxX), yOf(curve.max_speed)],
    ];
    const lineD = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
    const areaD = `${lineD} L${pts[3][0].toFixed(1)},${yOf(0).toFixed(1)} L${pts[0][0].toFixed(1)},${yOf(0).toFixed(1)} Z`;

    let markers = '';
    if (liveBpm > 0) {
      const lx = xOf(liveBpm).toFixed(1);
      const ly = yOf(mapSpeed(curve, liveBpm)).toFixed(1);
      markers += `<circle cx="${lx}" cy="${ly}" r="5" fill="var(--ok)" stroke="var(--surface)" stroke-width="1.5" />`;
    }
    if (effBpm > 0) {
      const ex = xOf(effBpm);
      const ey = yOf(mapSpeed(curve, effBpm));
      markers += `<polygon points="${ex.toFixed(1)},${(ey - 6).toFixed(1)} ${(ex + 6).toFixed(1)},${ey.toFixed(1)} ${ex.toFixed(1)},${(ey + 6).toFixed(1)} ${(ex - 6).toFixed(1)},${ey.toFixed(1)}" fill="var(--warn)" stroke="var(--surface)" stroke-width="1.5" />`;
    }

    svg.innerHTML = `${gridY}
      <path d="${areaD}" fill="var(--brand)" opacity="0.12" />
      <path d="${lineD}" fill="none" stroke="var(--brand)" stroke-width="2.4" stroke-linejoin="round" />
      ${markers}
      <text x="${xOf(curve.min_hr).toFixed(1)}" y="${H - 5}" text-anchor="middle" fill="var(--muted)" font-size="11">${curve.min_hr}</text>
      <text x="${xOf(curve.max_hr).toFixed(1)}" y="${H - 5}" text-anchor="middle" fill="var(--muted)" font-size="11">${curve.max_hr} bpm</text>`;
  }

  // Event listeners
  els.toggleRunBtn.addEventListener('click', async () => {
    if (!latestState) return;
    const url = latestState.running ? '/api/stop' : '/api/start';
    try {
      const st = await postJSON(url);
      render(st);
    } catch (err) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = err.message;
    }
  });

  els.modeAutoBtn.addEventListener('click', async () => {
    try {
      render(await postJSON('/api/mode', { mode: 'AUTO' }));
    } catch (err) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = err.message;
    }
  });

  els.modeManualBtn.addEventListener('click', async () => {
    try {
      render(await postJSON('/api/mode', { mode: 'MANUAL' }));
    } catch (err) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = err.message;
    }
  });

  els.manualSlider.addEventListener('pointerdown', () => { sliderDragging = true; });
  els.manualSlider.addEventListener('pointerup', () => { sliderDragging = false; });
  els.manualSlider.addEventListener('input', (e) => {
    const val = Number(e.target.value);
    els.manualSliderVal.textContent = `${val}%`;
    updateSliderProgress(val);
  });
  els.manualSlider.addEventListener('change', async (e) => {
    sliderDragging = false;
    try {
      render(await postJSON('/api/speed', { speed: Number(e.target.value) }));
    } catch (err) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = err.message;
    }
  });

  async function stepManual(delta) {
    const cur = latestState ? latestState.target_speed : Number(els.manualSlider.value);
    const next = Math.max(0, Math.min(100, cur + delta));
    try {
      render(await postJSON('/api/speed', { speed: next }));
    } catch (err) {
      els.alertBanner.hidden = false;
      els.alertBanner.textContent = err.message;
    }
  }

  els.speedDownBtn.addEventListener('click', () => stepManual(-10));
  els.speedUpBtn.addEventListener('click', () => stepManual(10));

  els.curveForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    els.saveStatus.textContent = 'Saving…';
    try {
      const st = await postJSON('/api/curve', {
        min_hr: Number(els.cfgMinHr.value),
        min_speed: Number(els.cfgMinSpeed.value),
        max_hr: Number(els.cfgMaxHr.value),
        max_speed: Number(els.cfgMaxSpeed.value),
        window_seconds: Number(els.cfgWindow.value),
        percentile: Number(els.cfgPercentile.value),
      });
      els.saveStatus.textContent = 'Saved';
      render(st);
    } catch (err) {
      els.saveStatus.textContent = err.message;
    }
  });

  // Initial fetch + SSE stream
  fetch('/api/state')
    .then((r) => r.json())
    .then(render)
    .catch(() => {});

  const es = new EventSource('/api/events');
  es.onopen = () => setBadge(els.connBadge, 'Live', 'ok');
  es.onerror = () => setBadge(els.connBadge, 'Reconnecting…', 'warn');
  es.onmessage = (ev) => {
    try {
      render(JSON.parse(ev.data));
    } catch {
      // Ignore malformed frame
    }
  };
})();
