# opendraft

An open-source, poor man's Wahoo Headwind alternative implemented in Go.

`opendraft` reads live heart rate (BPM) from a Bluetooth Low Energy (BLE) chest/arm strap (such as a Wahoo TICKR / TICKR FIT) and dynamically adjusts the speed of an indoor cycling fan via a **Shelly Dimmer 2** on your local Wi-Fi network—with zero cloud dependencies.

> [!CAUTION]
> **DANGER TO LIFE — HIGH VOLTAGE / MAINS ELECTRICITY & NO LIABILITY**
>
> - Working with mains electricity (**110V–240V AC**) is **extremely dangerous and potentially lethal**. Electric shock, arc flash, or improper wiring can cause **severe injury, death, or fire**.
> - **All mains wiring (including installing or connecting a Shelly Dimmer 2) must only be performed by a qualified, licensed electrician** in accordance with local electrical codes, with the circuit breaker turned off and verified dead before touching any conductors.
> - **Inductive motor warning:** Phase-cut dimmers like the Shelly Dimmer 2 are **not compatible with every AC fan motor**. Running an incompatible motor or stalling a motor at too-low brightness can overheat the motor or damage the dimmer. Verify your fan's compatibility and minimum starting speed.
> - **NO WARRANTY / NO LIABILITY:** This software and documentation are provided **"AS IS"**, without warranty of any kind, under the [MIT License](LICENSE). The authors and contributors assume **zero liability** for any personal injury, death, fire, or property/equipment damage arising from using this project or wiring electrical hardware. You use everything at your own sole risk.

---

## Features

- **BLE Heart Rate Monitoring:** Discovers BLE peripherals advertising the standard GATT Heart Rate Service (`0x180D` / `0x2A37`), tested with Wahoo TICKR FIT on macOS.
- **Shelly Dimmer 2 Local Control:** Discovers Shelly devices on your WLAN via mDNS (`_http._tcp` / `_shelly._tcp`) and controls brightness (`0–100%`) over local HTTP (`/light/0`, with Gen2 `/rpc/Light.Set` fallback)—no cloud or API keys required.
- **HR-to-Fan Control Loop:**
  - Explicit **START FAN** / **STOP FAN** control (stays off until you start, even while previewing live BPM).
  - Default linear curve: fixed **15%** at or below **120 bpm**, ramping linearly to **100%** at **165+ bpm** (editable live in the TUI).
  - Rate-limiting (`500ms` / `2%` deadband), `AUTO` / `MANUAL` override, and automatic `10s` HR-disconnect safety shutoff.
- **Bubble Tea TUI & Saved Hardware:** Remembers your selected BLE HR monitor, Shelly Dimmer, and curve in `~/.config/opendraft/config.json`, auto-reconnecting on subsequent launches.

## Quick Start

Prerequisites: Go 1.27+

```sh
# Run with real BLE + Shelly Dimmer 2 hardware
go run ./cmd/opendraft

# Run in simulated hardware mode (no BLE strap or Shelly required)
go run ./cmd/opendraft --mock
```

### Diagnostic Flags

```sh
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
  "max_speed": 100
}
```

## Documentation & AI-First Workflow

- **[Hardware & Network Setup Guide](docs/setup.md)** — How our setup works, Shelly Dimmer 2 notes, and troubleshooting.
- **[Feature Specifications](docs/features/README.md)** — All features are specified in `docs/features/` before or alongside implementation.
- **[AGENTS.md](AGENTS.md)** — Conventions and workflow for AI coding agents and contributors.

## License

[MIT](LICENSE)
