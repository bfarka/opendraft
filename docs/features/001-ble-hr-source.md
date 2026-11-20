# 001: BLE Heart Rate Source

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-20

## Problem

`opendraft` needs live heart rate (BPM) data from a Bluetooth Low Energy (BLE) chest strap (such as a Wahoo TICKR) without requiring proprietary dongles or cloud services.

## User Story

As an indoor cyclist, I want `opendraft` to scan for nearby BLE heart rate monitors and stream my live BPM so that my fan speed can react to my effort.

## Acceptance Criteria

- [ ] Define a hardware-agnostic `domain.HeartRateSource` interface.
- [ ] Scan for BLE peripherals and identify devices advertising Heart Rate Service (`0x180D`) or known HR monitor names.
- [ ] Connect to a selected BLE device, subscribe to GATT Heart Rate Measurement (`0x2A37`), and stream `domain.HRReading` values.
- [ ] Decode both 8-bit and 16-bit heart rate values per the Bluetooth SIG GATT specification, with table-driven unit tests.
- [ ] Support macOS (CoreBluetooth) and Linux (BlueZ) via `tinygo.org/x/bluetooth`.

## Design & Architecture

- `internal/domain`: `HRDevice`, `HRReading`, `HeartRateSource`.
- `internal/hr/ble`:
  - `parser.go`: pure-Go `ParseHeartRateMeasurement(data []byte) (uint8, error)` decoder.
  - `scanner.go`: `tinygo.org/x/bluetooth` implementation of `domain.HeartRateSource`.

## Out of Scope

- ANT+ heart rate straps.
- Multi-sensor fusion (power/speed/cadence).
- HRV / RR-interval analytics.

## Open Questions

- None.
