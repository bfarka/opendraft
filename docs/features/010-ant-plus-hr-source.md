# 010: ANT+ Heart Rate Source

- **Status:** Implemented
- **Created:** 2026-11-26
- **Updated:** 2026-11-26

## Problem

`opendraft` previously only supported Bluetooth Low Energy (BLE) heart rate monitors (`0x180D` / `0x2A37`). On a Raspberry Pi or indoor cycling setup with an ANT+ USB stick, users often have ANT+-only straps or prefer ANT+ reliability when BLE is busy with other trainer connections.

## User Story

As an indoor cyclist running `opendraft` on a Raspberry Pi with an ANT+ USB stick, I want `opendraft` to discover and stream heart rate measurements from ANT+ HR monitors alongside BLE monitors so that I can use either transport seamlessly.

## Acceptance Criteria

- [ ] Pure-Go Linux `usbfs` (`/dev/bus/usb`) transport supports Dynastream/Garmin ANT+ USB sticks (`0fcf:1008`, `0fcf:1009`) with zero CGo or `libusb` dependencies.
- [ ] Non-Linux platforms (e.g., macOS) compile and run without error, gracefully skipping ANT+ USB discovery.
- [ ] Composite `HeartRateSource` scans BLE and ANT+ simultaneously, presents a unified HR device list with `[BLE]` / `[ANT+]` transport labels, and routes `Connect` to the matching transport.
- [ ] Failure/absence of one transport (e.g. no ANT+ stick plugged in) does not block the other transport.
- [ ] `config.Config` persists transport-neutral `hr_device_id` / `hr_device_name` (e.g. `ant:12345`, `ble:AA:BB:...`) while preserving backwards-compatible fallback from legacy `ble_device_id` / `ble_device_name`.
- [ ] TUI, WebUI, `--debug-scan`, and `--debug-hr` support ANT+ devices.

## Design & Architecture

- **`internal/hr/ant`**:
  - `protocol.go`: ANT+ message framing (`0xA4` sync, length, msgID, payload, XOR checksum), command builders (`ResetSystem`, `SetNetworkKey` with ANT+ public key `B9 A5 21 FB BD 72 C3 45`, `AssignChannel`, `SetChannelID` device type `120`, `SetChannelPeriod` `8070`, `SetChannelRFFreq` `57`, `SetChannelSearchTimeout`, `LibConfig 0xE0`, `OpenRxScanMode`, `OpenChannel`, `CloseChannel`), and HRM Broadcast Data (`0x4E`) parser with extended Channel ID & RSSI extraction.
  - `usb_linux.go` / `usb_other.go`: Pure-Go `usbfs` bulk I/O via `/sys/bus/usb/devices` and `/dev/bus/usb/BBB/DDD` (`USBDEVFS_CLAIMINTERFACE`, `USBDEVFS_BULK`).
  - `source.go`: `domain.HeartRateSource` implementation.
- **`internal/hr/multi`**:
  - Composite `domain.HeartRateSource` running multiple named sources concurrently and routing `Connect` by `ble:` / `ant:` prefix (with unprefixed IDs defaulting to `ble` for backwards compatibility).
- **`internal/config`**:
  - `hr_device_id` / `hr_device_name` fields with `SavedHRDeviceID()` / `SavedHRDeviceName()` helpers.

## Out of Scope

- ANT+ power meters, cadence sensors, or FE-C trainer control.
- macOS/Windows native USB driver support for ANT+ sticks.
- ANT+ pairing/calibration beyond standard HRM broadcast reception.
