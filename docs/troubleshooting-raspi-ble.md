# Troubleshooting BLE Discovery on Raspberry Pi

## Context

`opendraft` uses `tinygo.org/x/bluetooth` for BLE heart rate discovery (`internal/hr/ble/scanner.go`), which on Linux communicates with **BlueZ (`org.bluez`) over the system D-Bus** using adapter `hci0`.

Note: `Source.Scan` in `internal/hr/ble/scanner.go` currently ignores the error returned by `s.adapter.Scan(...)`, so if BlueZ rejects the scan, the TUI silently shows `"No BLE HR devices found yet"`. The `--debug-scan` flag (`cmd/opendraft/debug_scan.go`), however, prints `Enable` and `Scan` errors as well as all raw BLE advertisements without filtering.

## Step 1: Run `opendraft --debug-scan`

Run the built-in 5-second diagnostic scan from the `opendraft` directory (or wherever the binary is located):

```sh
./opendraft --debug-scan
```

Check whether it prints `[BLE] Enable error: ...`, `[BLE] Scan error: ...`, or any `[BLE] addr=...` lines.

## Step 2: Check BlueZ, `rfkill`, Adapter State, and Permissions

Run the following diagnostic commands on the Raspberry Pi:

1. **Verify `bluetoothd` is running:**
   ```sh
   sudo systemctl status bluetooth
   ```
   If inactive, enable and start it:
   ```sh
   sudo systemctl enable --now bluetooth
   ```

2. **Check `rfkill` and adapter power (`hci0`):**
   `tinygo.org/x/bluetooth` requires `hci0` (`/org/bluez/hci0`) to exist and `org.bluez.Adapter1.Powered` to be `true`:
   ```sh
   rfkill list
   sudo rfkill unblock bluetooth
   bluetoothctl power on
   bluetoothctl show
   ```
   Confirm `Powered: yes` and that `hci0` is present (`hciconfig -a` or `btmgmt info`).

3. **Check D-Bus permissions:**
   Non-root users may lack permission to talk to `org.bluez` on the system D-Bus. Test with `sudo`:
   ```sh
   sudo ./opendraft --debug-scan
   ```
   If it works with `sudo` but not as the regular user, add the user to the `bluetooth` group (and start a new login session):
   ```sh
   sudo usermod -aG bluetooth "$USER"
   ```

4. **Test raw BlueZ BLE discovery:**
   ```sh
   bluetoothctl --timeout 10 scan on
   ```
   - If `bluetoothctl` sees nothing, verify the HR strap is awake/worn and **not already connected** to another device (phone, watch, bike computer), as most BLE straps stop advertising once connected.
   - If `bluetoothctl` sees the strap but `opendraft` TUI does not, check the `--debug-scan` output to see whether `has0x180D=true` or the advertised `name` contains `TICKR`, `HRM`, or `HEART` (`looksLikeHRMonitor` in `internal/hr/ble/scanner.go`).
