# 003: Fan Control Loop & Linear HR Mapping

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-20

## Problem

Raw heart rate readings arrive every second and fluctuate; sending every reading directly to a WLAN dimmer would spam the device and lack manual override or disconnect safety.

## User Story

As an indoor cyclist, I want `opendraft` to map my heart rate to a fan percentage, rate-limit updates, let me override manually, and handle HR sensor disconnects safely.

## Acceptance Criteria

- [x] Define a `domain.SpeedMapper` interface so custom HR-to-speed curves can be swapped in later.
- [x] Implement `LinearMapper` mapping `0 < bpm <= MinHR` to fixed `MinSpeed` (default `120 bpm -> 15%`), `MinHR..MaxHR` linearly to `MinSpeed..MaxSpeed`, and `>= MaxHR` to `MaxSpeed` (default `165 bpm -> 100%`).
- [x] Support explicit `Start` / `Stop` state (default stopped so the fan stays off until started, while still tracking live BPM).
- [x] Support `ModeAuto` and `ModeManual` (manual override pauses HR-driven updates and allows direct `0–100%` control).
- [x] Rate-limit dimmer writes (minimum delta threshold and minimum interval) to avoid flooding the WLAN dimmer.
- [x] Handle HR signal timeout (e.g., 10s without readings) by notifying state and safely turning off or holding per policy.
- [x] Table-driven unit tests for `LinearMapper` and `control.Loop`.

## Design & Architecture

- `internal/control/mapper.go`: `LinearMapper`.
- `internal/control/loop.go`: `Loop` coordinating HR channel, `SpeedMapper`, `FanController`, mode state, rate-limiting, and status snapshots.

## Out of Scope

- Custom non-linear / exponential / hysteresis curves (planned as a follow-up feature).

## Open Questions

- None.
