# 007: Local WebUI Dashboard & Control API

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-21

## Problem

While riding indoors, `opendraft`'s TUI runs in a terminal window that may be minimized, behind a training app (Zwift/TrainerRoad), or inconvenient to read at a glance. Users want a clean, local web interface they can open in any browser to see how much the fan is running (live fan speed %, target speed, mode), live & percentile heart rate, the rolling HR history chart, the HR-to-fan curve, and basic controls (Start/Stop, Auto/Manual speed, curve tuning).

## User Story

As an indoor cyclist, I want a local WebUI served by `opendraft` so that I can monitor fan speed, heart rate, and control settings from a browser while riding.

## Acceptance Criteria

- [x] `opendraft` starts an embedded HTTP server on `--web-port` / `--web-addr` (default port `8081`, configurable via `web_port` in `config.json`; `0` or empty `--web-addr ""` disables it) alongside the TUI, or standalone with `--headless` (useful with `--mock` or saved devices), and displays the resolved LAN URL in the TUI. If WebUI is enabled and the port is already in use, `opendraft` logs an error message and exits with a non-zero status.
- [x] `GET /api/state` returns a JSON snapshot of the live control loop state, rolling HR history, curve configuration, and connected/selected HR & fan devices.
- [x] `GET /api/events` streams live state updates via Server-Sent Events (SSE) whenever readings/state change (and a 2s heartbeat).
- [x] Control endpoints (`POST /api/start`, `POST /api/stop`, `POST /api/mode`, `POST /api/speed`, `POST /api/curve`) allow starting/stopping the fan, switching AUTO/MANUAL, setting manual speed (0–100%), and updating + persisting the HR-to-fan curve & percentile window.
- [x] Device discovery & selection endpoints (`POST /api/scan`, `POST /api/scan/stop`, `POST /api/devices/hr`, `POST /api/devices/fan`) allow scanning for BLE HR monitors and Shelly dimmers (with live results streamed via `State.Discovery` over `/api/state` and `/api/events`), connecting to a BLE HR monitor, selecting a Shelly dimmer, and persisting hardware selections to `config.json`.
- [x] Embedded single-page WebUI (`GET /`) displays:
  - Prominent fan speed gauge / progress bar (applied % vs target %, running/stopped badge, AUTO/MANUAL badge)
  - Live BPM, effective percentile BPM (with warm-up count), and connected BLE/Fan device info
  - Device Discovery modal (`Devices` / `Scan` button) to scan for BLE HR monitors and Shelly dimmers and connect/select them directly from the browser
  - Interactive controls: Start/Stop button, Auto/Manual toggle, Manual speed slider (0–100%), and Curve & Window settings form
  - SVG charts for rolling HR history (with percentile line) and HR-to-fan curve (with live & effective HR markers)
  - Automatic light/dark theme support (`color-scheme: light dark`) and responsive layout
- [x] Unit tests cover the HTTP handlers, SSE stream, state bridge thread-safety, device discovery/selection, and curve validation.

## Design & Architecture

- **`internal/web/bridge.go`**: Thread-safe `Bridge` connecting `control.Loop`, `config.Config`, and device metadata to the HTTP server. Supports `Subscribe()`/`Unsubscribe()` channels for SSE fan-out and `Notify()` on state/HR updates.
- **`internal/web/server.go`**: Standard-library `net/http` server bound via `net.Listen("tcp4", addr)` (IPv4-only for reliable LAN reachability from mobile devices on macOS) with embedded static assets (`//go:embed static/*`), JSON API endpoints, and SSE `/api/events` endpoint.
- **`internal/web/static/`**: Vanilla HTML, CSS, and JS (`index.html`, `style.css`, `app.js`) with no build step or external CDN dependencies.
- **`internal/tui/model.go`**: Optional `*web.Bridge` hook so TUI updates (`HandleHR`, `CheckTimeout`, device selection, curve edits) notify the WebUI in real time, and WebUI curve edits stay synced with `m.cfg`.
- **`cmd/opendraft/main.go`**: Add `--web-port` (overrides `config.WebPort`, default `8081`), `--web-addr` (default `:8081`), and `--headless` (run without TUI, auto-connecting to saved/mock devices), failing fast with a logged error if the WebUI port is already in use.

## Out of Scope

- Authentication / TLS (intended for localhost or trusted LAN).
- Multi-user or cloud relay.
