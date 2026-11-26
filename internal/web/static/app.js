(() => {
  const $ = (id) => document.getElementById(id);

  const els = {
    appVersion: $('app-version'),
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
    openThresholdsBtn: $('open-thresholds-btn'),
    thresholdsDialog: $('thresholds-dialog'),
    closeThresholdsBtn: $('close-thresholds-btn'),
    cancelThresholdsBtn: $('cancel-thresholds-btn'),
    thresholdsDialogForm: $('thresholds-dialog-form'),
    saveThresholdsBtn: $('save-thresholds-btn'),
    dlgMinHr: $('dlg-min-hr'),
    dlgMinSpeed: $('dlg-min-speed'),
    dlgMaxHr: $('dlg-max-hr'),
    dlgMaxSpeed: $('dlg-max-speed'),
    dlgWindow: $('dlg-window'),
    dlgPercentile: $('dlg-percentile'),
    dlgError: $('dlg-error'),
    openDevicesBtn: $('open-devices-btn'),
    cardScanBtn: $('card-scan-btn'),
    devicesDialog: $('devices-dialog'),
    closeDevicesBtn: $('close-devices-btn'),
    scanDevicesBtn: $('scan-devices-btn'),
    stopScanBtn: $('stop-scan-btn'),
    scanStatusText: $('scan-status-text'),
    devicesError: $('devices-error'),
    discHrBadge: $('disc-hr-badge'),
    discFanBadge: $('disc-fan-badge'),
    discHrList: $('disc-hr-list'),
    discFanList: $('disc-fan-list'),
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

    if (els.appVersion && st.version) {
      els.appVersion.textContent = st.version;
    }

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
    renderDiscovery(st);
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

  function openThresholdsDialog() {
    const c = latestState?.curve;
    els.dlgMinHr.value = c ? c.min_hr : els.cfgMinHr.value;
    els.dlgMinSpeed.value = c ? c.min_speed : els.cfgMinSpeed.value;
    els.dlgMaxHr.value = c ? c.max_hr : els.cfgMaxHr.value;
    els.dlgMaxSpeed.value = c ? c.max_speed : els.cfgMaxSpeed.value;
    els.dlgWindow.value = c ? c.window_seconds : els.cfgWindow.value;
    els.dlgPercentile.value = c ? c.percentile : els.cfgPercentile.value;
    els.dlgError.textContent = '';
    els.thresholdsDialog.showModal();
  }

  function enableLightDismissFallback(dialog) {
    if ('closedBy' in HTMLDialogElement.prototype) return;
    const outside = (e) => {
      const r = dialog.getBoundingClientRect();
      return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
    };
    let pressedOutside = false;
    dialog.addEventListener('pointerdown', (e) => {
      pressedOutside = outside(e);
    });
    dialog.addEventListener('pointerup', (e) => {
      if (pressedOutside && outside(e)) {
        dialog.close();
      }
    });
  }

  function renderDiscovery(st) {
    const disc = st.discovery || {};
    const bleScanning = Boolean(disc.ble_scanning);
    const fanScanning = Boolean(disc.fan_scanning);
    const anyScanning = bleScanning || fanScanning;

    els.scanDevicesBtn.disabled = anyScanning;
    els.scanDevicesBtn.textContent = anyScanning ? 'Scanning…' : 'Scan Devices';
    els.stopScanBtn.hidden = !anyScanning;

    if (anyScanning) {
      els.scanStatusText.textContent = 'Scanning HR (BLE / ANT+) & Shelly mDNS (15s)…';
    } else {
      els.scanStatusText.textContent = 'Scan for nearby BLE / ANT+ heart rate straps and Shelly dimmers on your LAN.';
    }

    setBadge(els.discHrBadge, bleScanning ? 'Scanning…' : 'Idle', bleScanning ? 'brand' : 'muted');
    setBadge(els.discFanBadge, fanScanning ? 'Scanning…' : 'Idle', fanScanning ? 'brand' : 'muted');

    // HR devices list
    els.discHrList.replaceChildren();
    const hrs = disc.hr_devices || [];
    if (disc.ble_error) {
      const li = document.createElement('li');
      li.className = 'discovery-empty';
      li.textContent = `BLE error: ${disc.ble_error}`;
      els.discHrList.appendChild(li);
    } else if (hrs.length === 0) {
      const li = document.createElement('li');
      li.className = 'discovery-empty';
      li.textContent = bleScanning ? 'Listening for BLE / ANT+ heart rate sensors…' : 'No HR monitors found yet.';
      els.discHrList.appendChild(li);
    } else {
      for (const dev of hrs) {
        const li = document.createElement('li');
        li.className = 'discovery-item';

        const info = document.createElement('div');
        info.className = 'discovery-item-info';
        const nameEl = document.createElement('span');
        nameEl.className = 'discovery-item-name';
        nameEl.textContent = dev.name || dev.id;
        const metaEl = document.createElement('span');
        metaEl.className = 'discovery-item-meta';
        const transportLabel = dev.transport ? `[${dev.transport.toUpperCase()}] ` : '';
        metaEl.textContent = `${transportLabel}${dev.rssi} dBm`;
        info.append(nameEl, metaEl);
        li.appendChild(info);

        const isConnected = Boolean(st.hr_device && st.hr_device.id === dev.id && st.hr_connected);
        if (isConnected) {
          const badge = document.createElement('span');
          badge.className = 'badge badge-ok';
          badge.textContent = 'Connected';
          li.appendChild(badge);
        } else {
          const btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'btn btn-primary btn-sm';
          btn.textContent = 'Connect';
          btn.addEventListener('click', async () => {
            els.devicesError.textContent = '';
            btn.disabled = true;
            btn.textContent = 'Connecting…';
            try {
              const next = await postJSON('/api/devices/hr', { id: dev.id });
              render(next);
            } catch (err) {
              els.devicesError.textContent = err.message;
              btn.disabled = false;
              btn.textContent = 'Connect';
            }
          });
          li.appendChild(btn);
        }
        els.discHrList.appendChild(li);
      }
    }

    // Fan devices list
    els.discFanList.replaceChildren();
    const fans = disc.fan_devices || [];
    if (disc.fan_error) {
      const li = document.createElement('li');
      li.className = 'discovery-empty';
      li.textContent = `mDNS error: ${disc.fan_error}`;
      els.discFanList.appendChild(li);
    } else if (fans.length === 0) {
      const li = document.createElement('li');
      li.className = 'discovery-empty';
      li.textContent = fanScanning ? 'Browsing LAN via mDNS (_http._tcp / _shelly._tcp)…' : 'No Shelly dimmers found yet.';
      els.discFanList.appendChild(li);
    } else {
      for (const dev of fans) {
        const li = document.createElement('li');
        li.className = 'discovery-item';

        const info = document.createElement('div');
        info.className = 'discovery-item-info';
        const nameEl = document.createElement('span');
        nameEl.className = 'discovery-item-name';
        nameEl.textContent = dev.name || dev.id;
        const metaEl = document.createElement('span');
        metaEl.className = 'discovery-item-meta';
        metaEl.textContent = dev.ip ? `${dev.ip}:${dev.port || 80}` : dev.id;
        info.append(nameEl, metaEl);
        li.appendChild(info);

        const isSelected = Boolean(st.fan_device && st.fan_device.id === dev.id);
        if (isSelected) {
          const badge = document.createElement('span');
          badge.className = 'badge badge-ok';
          badge.textContent = 'Selected';
          li.appendChild(badge);
        } else {
          const btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'btn btn-primary btn-sm';
          btn.textContent = 'Select';
          btn.addEventListener('click', async () => {
            els.devicesError.textContent = '';
            btn.disabled = true;
            btn.textContent = 'Selecting…';
            try {
              const next = await postJSON('/api/devices/fan', { id: dev.id });
              render(next);
            } catch (err) {
              els.devicesError.textContent = err.message;
              btn.disabled = false;
              btn.textContent = 'Select';
            }
          });
          li.appendChild(btn);
        }
        els.discFanList.appendChild(li);
      }
    }
  }

  async function startDeviceScan() {
    els.devicesError.textContent = '';
    els.scanDevicesBtn.disabled = true;
    try {
      const st = await postJSON('/api/scan');
      render(st);
    } catch (err) {
      els.devicesError.textContent = err.message;
      els.scanDevicesBtn.disabled = false;
    }
  }

  function openDevicesDialog(autoScan) {
    els.devicesError.textContent = '';
    if (latestState) {
      renderDiscovery(latestState);
    }
    els.devicesDialog.showModal();
    const disc = latestState && latestState.discovery;
    const scanning = disc && (disc.ble_scanning || disc.fan_scanning);
    const empty = !disc || ((disc.hr_devices || []).length === 0 && (disc.fan_devices || []).length === 0);
    if ((autoScan || empty) && !scanning) {
      startDeviceScan();
    }
  }

  els.openThresholdsBtn.addEventListener('click', openThresholdsDialog);
  els.closeThresholdsBtn.addEventListener('click', () => els.thresholdsDialog.close());
  els.cancelThresholdsBtn.addEventListener('click', () => els.thresholdsDialog.close());
  enableLightDismissFallback(els.thresholdsDialog);

  els.openDevicesBtn.addEventListener('click', () => openDevicesDialog(false));
  els.cardScanBtn.addEventListener('click', () => openDevicesDialog(true));
  els.closeDevicesBtn.addEventListener('click', () => els.devicesDialog.close());
  els.scanDevicesBtn.addEventListener('click', startDeviceScan);
  els.stopScanBtn.addEventListener('click', async () => {
    els.devicesError.textContent = '';
    try {
      render(await postJSON('/api/scan/stop'));
    } catch (err) {
      els.devicesError.textContent = err.message;
    }
  });
  enableLightDismissFallback(els.devicesDialog);

  els.thresholdsDialogForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    els.dlgError.textContent = '';
    els.saveThresholdsBtn.disabled = true;
    try {
      const st = await postJSON('/api/curve', {
        min_hr: Number(els.dlgMinHr.value),
        min_speed: Number(els.dlgMinSpeed.value),
        max_hr: Number(els.dlgMaxHr.value),
        max_speed: Number(els.dlgMaxSpeed.value),
        window_seconds: Number(els.dlgWindow.value),
        percentile: Number(els.dlgPercentile.value),
      });
      curveInitialized = false;
      els.saveStatus.textContent = 'Saved';
      render(st);
      els.thresholdsDialog.close();
    } catch (err) {
      els.dlgError.textContent = err.message;
    } finally {
      els.saveThresholdsBtn.disabled = false;
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
