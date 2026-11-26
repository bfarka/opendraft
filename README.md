# opendraft

An open-source, poor man's Wahoo Headwind alternative implemented in Go.

`opendraft` reads live heart rate (BPM) from a Bluetooth Low Energy (BLE) or ANT+ chest/arm strap (such as a Wahoo TICKR / TICKR FIT or Garmin HRM) and dynamically adjusts the speed of an indoor cycling fan via a **Shelly Dimmer 2** on your local Wi-Fi network—with zero cloud dependencies.

> [!CAUTION]
> **DANGER TO LIFE — HIGH VOLTAGE / MAINS ELECTRICITY & NO LIABILITY**
>
> - Working with mains electricity (**110V–240V AC**) is **extremely dangerous and potentially lethal**. Electric shock, arc flash, or improper wiring can cause **severe injury, death, or fire**.
> - **All mains wiring (including installing or connecting a Shelly Dimmer 2) must only be performed by a qualified, licensed electrician** in accordance with local electrical codes, with the circuit breaker turned off and verified dead before touching any conductors.
> - **Inductive motor warning:** Phase-cut dimmers like the Shelly Dimmer 2 are **not compatible with every AC fan motor**. Running an incompatible motor or stalling a motor at too-low brightness can overheat the motor or damage the dimmer. Verify your fan's compatibility and minimum starting speed.
> - **NO WARRANTY / NO LIABILITY:** This software and documentation are provided **"AS IS"**, without warranty of any kind, under the [MIT License](LICENSE). The authors and contributors assume **zero liability** for any personal injury, death, fire, or property/equipment damage arising from using this project or wiring electrical hardware. You use everything at your own sole risk.

---

## Features

- **BLE & ANT+ Heart Rate Monitoring:** Discovers BLE peripherals advertising the standard GATT Heart Rate Service (`0x180D` / `0x2A37`) as well as ANT+ HR straps via a USB ANT+ stick (`0fcf:1008` / `0fcf:1009`) on Linux/Raspberry Pi (pure-Go `usbfs`, no `libusb`/CGo required). See [docs/setup.md](docs/setup.md) for the Raspberry Pi `udev` rule.
- **Shelly Dimmer 2 Local Control:** Discovers Shelly devices on your WLAN via mDNS (`_http._tcp` / `_shelly._tcp`) and controls brightness (`0–100%`) over local HTTP (`/light/0`, with Gen2 `/rpc/Light.Set` fallback)—no cloud or API keys required.
- **HR-to-Fan Control Loop:**
  - Explicit **START FAN** / **STOP FAN** control (stays off until you start, even while previewing live BPM).
  - Default linear curve: fixed **15%** at or below **120 bpm**, ramping linearly to **100%** at **165+ bpm** (editable live in the TUI).
  - Rate-limiting (`500ms` / `2%` deadband), `AUTO` / `MANUAL` override, and automatic `10s` HR-disconnect safety shutoff.
- **Bubble Tea TUI & Saved Hardware:** Remembers your selected BLE HR monitor, Shelly Dimmer, and curve in `~/.config/opendraft/config.json`, auto-reconnecting on subsequent launches.
- **Local WebUI & Live SSE Dashboard:** Built-in browser dashboard (default `:8081`, IPv4) showing live fan speed %, target speed, live & percentile HR, rolling HR history and HR-to-fan curve SVG charts, plus Start/Stop, Auto/Manual speed, and curve/threshold controls from your computer, phone, or tablet on the same LAN.

## Quick Start

Prerequisites: Go 1.27+

```sh
# Run with real BLE + Shelly Dimmer 2 hardware
go run ./cmd/opendraft

# Run in simulated hardware mode (no BLE strap or Shelly required)
go run ./cmd/opendraft --mock

# Run headless (WebUI only, no TUI) with saved or mock devices
go run ./cmd/opendraft --headless --mock
```

### WebUI & Diagnostic Flags

```sh
# Bind WebUI to a custom port or address (default port 8081, 0 or "" disables WebUI)
go run ./cmd/opendraft --web-port 9090
go run ./cmd/opendraft --web-addr :9090

# Disable WebUI
go run ./cmd/opendraft --web-port 0
go run ./cmd/opendraft --web-addr ""

# Print raw BLE advertisements and Shelly mDNS entries for 5 seconds
go run ./cmd/opendraft --debug-scan

# Connect to the first BLE HR monitor and print live BPM for 10 seconds
go run ./cmd/opendraft --debug-hr

# Use a custom config file path
go run ./cmd/opendraft --config /path/to/config.json
```

## TUI Controls

### Discovery View
- `Tab` / `←` / `→` — Switch between **BLE Heart Rate Monitors** and **Shelly Dimmers (mDNS)**
- `↑` / `↓` (or `j` / `k`) — Move selection cursor
- `Enter` — Connect to BLE HR monitor / select Shelly Dimmer
- `r` — Rescan BLE & mDNS
- `d` — Go to Dashboard
- `q` — Quit

### Dashboard View
- `Tab` / `←` / `→` — Switch button focus between **`START FAN` / `STOP FAN`** and **`SCAN DEVICES`**
- `Space` / `Enter` (or mouse click) — Activate the focused button
- `c` — Edit HR curve (`Entry BPM`, `Entry Fan %`, `Max BPM`, `Max Fan %`)
- `a` — Toggle `AUTO` / `MANUAL` mode
- `+` / `-` (or `↑` / `↓`) — Adjust manual fan speed by `5%`
- `0` — Set manual fan speed to `0%`
- `s` — Jump directly to **Discovery / Scan**
- `q` — Stop fan and quit

## Local WebUI

When running, `opendraft` serves an embedded WebUI on `--web-addr` (default `:8081`, IPv4) and displays the resolved LAN URL (e.g. `http://192.168.x.x:8081`) in the TUI.

- **Dashboard (`GET /`):** Live fan speed gauge, Start/Stop, Auto/Manual toggle, manual speed slider (`0–100%`), rolling HR history chart, HR-to-fan curve chart, and curve/window editor.
- **API & SSE:**
  - `GET /api/state` — JSON snapshot of control loop state, HR history, curve config, and devices
  - `GET /api/events` — Server-Sent Events (SSE) stream of live state updates
  - `POST /api/start`, `POST /api/stop` — Start/stop fan control
  - `POST /api/mode` (`{"mode":"AUTO"|"MANUAL"}`) — Switch control mode
  - `POST /api/speed` (`{"speed":0..100}`) — Set manual fan speed
  - `POST /api/curve` — Update and persist HR-to-fan curve & rolling window settings

## Configuration

Settings are saved automatically to `~/.config/opendraft/config.json`:

```json
{
  "ble_device_id": "dba1e49d-4fb3-4187-e610-77b59cc93f5d",
  "ble_device_name": "TICKR FIT 4C31",
  "fan_device_id": "shellydimmer2-485519F35BA7",
  "fan_device_name": "shellydimmer2-485519F35BA7 (Shelly)",
  "fan_host": "192.168.193.129:80",
  "fan_model": "shelly",
  "min_hr": 120,
  "max_hr": 165,
  "min_speed": 15,
  "max_speed": 100,
  "web_port": 8081
}
```

## Documentation & AI-First Workflow

- **[Hardware & Network Setup Guide](docs/setup.md)** — How our setup works, Shelly Dimmer 2 notes, and troubleshooting.
- **[Feature Specifications](docs/features/README.md)** — All features are specified in `docs/features/` before or alongside implementation.
- **[AGENTS.md](AGENTS.md)** — Conventions and workflow for AI coding agents and contributors.

## Contributing

Contributions for **additional hardware, protocols, and features are very welcome**!

Because `opendraft` separates domain interfaces in [`internal/domain/domain.go`](internal/domain/domain.go) from hardware adapters, it's straightforward to add:
- **Other fan / dimmer backends** (`domain.FanDiscoverer` + `domain.FanController`) — e.g. Tasmota, ESPHome, MQTT, Home Assistant, smart plugs, or PWM controllers.
- **Other metric sources** (`domain.HeartRateSource` or new mappers) — e.g. ANT+ straps, smart trainer power/speed (FTMS), or custom fan curves (`domain.SpeedMapper`).

To contribute, start with a short feature spec in `docs/features/` using [`docs/features/_template.md`](docs/features/_template.md), include table-driven unit tests, and run `go vet ./...` and `go test -race ./...` before opening a PR.

Enable the local Git hooks once after cloning:

```sh
make hooks
```

Commits and PR titles follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `feat!:`, `chore:`, etc.). On `main`, [Release Please](https://github.com/googleapis/release-please) maintains a Release PR with generated release notes and `CHANGELOG.md`; merging that PR creates the SemVer tag and GitHub Release.

## License

[MIT](LICENSE)
