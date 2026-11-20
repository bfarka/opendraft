# 005: Saved Hardware Auto-Connect & Scan Button

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-20

## Problem

Re-selecting the same BLE HR monitor and Shelly Dimmer from the Discovery screen on every launch is repetitive once hardware has been chosen.

## User Story

As an indoor cyclist, I want `opendraft` to remember my chosen HR monitor and Shelly Dimmer in `~/.config/opendraft/config.json`, start directly on the Dashboard and reconnect automatically, and give me a Scan button when I want to switch devices.

## Acceptance Criteria

- [x] Persist `FanDeviceID`, `FanDeviceName`, `FanHost`, `FanModel`, `BLEDeviceID`, and `BLEDeviceName` in `config.Config`.
- [x] When both `BLEDeviceID` and `FanDeviceID` are present in config, start directly on `viewDashboard` instead of `viewDiscovery`.
- [x] Reconstruct the saved `domain.FanDevice` immediately from config and auto-scan/auto-connect to the saved `BLEDeviceID` in the background.
- [x] Render a keyboard-navigable `[ SCAN DEVICES ]` button on the Dashboard (`Tab`/`←`/`→` + `Enter`/`Space` or `s` or mouse click) to switch to Discovery and rescan for other devices.
- [x] Unit tests covering startup with saved hardware vs. empty config.

## Design & Architecture

- `internal/config/config.go`: `HasSavedDevices()` and `SavedFanDevice()` helpers.
- `internal/tui/model.go`: If `cfg.HasSavedDevices()`, initialize `selectedFan`, `fanCtrl`, `view = viewDashboard`, and auto-connect when `bleDeviceMsg` matches `cfg.BLEDeviceID`.
- `internal/tui/dashboard.go`: Render `[ SCAN DEVICES ]` button next to `[ START / STOP ]`.

## Out of Scope

- Multi-fan orchestration.

## Open Questions

- None.
