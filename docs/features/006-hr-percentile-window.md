# 006: 120s HR Percentile Window & Dashboard Visualization

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-20

## Problem

Instantaneous heart rate (BPM) can fluctuate briefly from sensor noise or short surges, causing the fan speed to oscillate unnecessarily. Using a rolling time window (default 120 seconds) and taking a high percentile (default 80th percentile) smooths out brief dips while still responding promptly to sustained effort—without storing unbounded history or allocating per reading.

## User Story

As an indoor cyclist, I want `opendraft` to compute the 80th percentile of my heart rate over the last 120 seconds (in a memory-efficient ring buffer) to drive the fan curve, and visualize both the 120s HR history and the HR-to-fan curve on the TUI dashboard, so I can see how my effort maps to fan speed.

## Acceptance Criteria

- [x] `config.Config` includes `WindowSeconds` (default `120`) and `Percentile` (default `80`), persisted in `config.json` and editable in the TUI curve editor (`c`).
- [x] A memory-efficient `HRWindow` uses a fixed 1-second ring buffer (`[]uint8`, 1 byte/sec) and a `[256]uint16` BPM frequency histogram (zero allocations per reading, `O(1)` insert, `O(256)` percentile without sorting).
- [x] `control.Loop` feeds incoming HR readings (every 1s) into `HRWindow`, computes the configured percentile over available samples during warm-up, and maps that percentile BPM through `SpeedMapper` in `AUTO` mode.
- [x] On HR timeout/disconnect (`CheckTimeout`), the window is cleared so stale HR does not carry over on reconnect.
- [x] TUI dashboard displays both live HR and effective percentile HR (with warm-up count), a 120s HR history chart with a percentile marker, and an HR-to-fan curve plot with live/percentile markers.
- [x] Table-driven unit tests cover `HRWindow`, `control.Loop` percentile integration, `config.Load`/`Save` defaults, and TUI rendering/editing.

## Design & Architecture

- **`internal/config`**: Add `WindowSeconds int` (`json:"window_seconds"`) and `Percentile int` (`json:"percentile"`) with `Default()` values `120` and `80`. If an existing `config.json` omits these fields (zero values), `Load` preserves defaults (`120` / `80`).
- **`internal/control/window.go`**: `HRWindow` with:
  - `slots []uint8` (length `windowSeconds`, 1 byte per second; `0` means empty/no reading for that second)
  - `hist [256]uint16` (512 bytes frequency table for BPM `1..255`)
  - `lastSec int64`, `head int`, `count int`, `hasData bool`
  - `Add(bpm uint8, ts time.Time)`: advances `lastSec` to `ts.Unix()`, evicting skipped slots from `hist`, and writes `bpm` into the current second's slot.
  - `Percentile(p int) uint8`: scans `hist[1..255]` to find the `ceil(p/100 * count)`-th element in `O(256)` time and zero allocations.
  - `History(dst []uint8) []uint8`: returns the chronological 1-second slots (oldest → newest) for visualization.
  - `Reset()`: clears slots, histogram, and counters.
- **`internal/control/loop.go`**: Integrates `HRWindow`, exposes `EffectiveBPM`, `WindowCount`, `WindowSeconds`, `Percentile`, and `History` in `Snapshot` (or `History()` method) for the TUI.
- **`internal/tui/dashboard.go` & `internal/tui/chart.go`**:
  - Curve editor (`c`) extended to 6 fields: Entry BPM, Entry Fan %, Max BPM, Max Fan %, Window (s), Percentile (%).
  - Dashboard renders a 120s HR history sparkline/chart with P80 reference and an HR-to-Fan curve plot with live (`●`) and P80 (`◆`) markers.

## Out of Scope

- Non-linear (e.g. cubic/exponential) fan curves.
- Persistent historical workout logging to disk.
