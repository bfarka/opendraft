# 002: Shelly Dimmer mDNS & HTTP Controller

- **Status:** Implemented
- **Created:** 2026-11-20
- **Updated:** 2026-11-21

## Problem

`opendraft` needs to discover a Shelly Dimmer (e.g. Shelly Dimmer 2) on the local network and adjust its speed/brightness (`0–100%`) over local HTTP without cloud round-trips or encryption keys.

## User Story

As an indoor cyclist, I want `opendraft` to automatically discover my Shelly Dimmer via mDNS and control its percentage over LAN so that I can use an off-the-shelf fan with a smart dimmer.

## Acceptance Criteria

- [x] Define a hardware-agnostic `domain.FanController` interface (`SetSpeed`, `Off`, `Info`) and `domain.FanDiscoverer` interface.
- [x] Discover Shelly devices via mDNS (`_http._tcp` and `_shelly._tcp`, matching `shelly*` instances/hosts/TXT records) and filter to dimmer models only (via `/shelly` HTTP capability probe with mDNS name/TXT fallback).
- [x] Control Shelly Dimmer 2 via local HTTP GET (`/light/0?turn=on&brightness=X` and `/light/0?turn=off`), with Gen2 `/rpc/Light.Set` fallback.
- [x] Unit tests for mDNS entry parsing and HTTP client behavior.

## Design & Architecture

- `internal/domain`: `FanDevice`, `FanDiscoverer`, `FanController`.
- `internal/fan/shelly`:
  - `discovery.go`: mDNS `_http._tcp` & `_shelly._tcp` browser with `/shelly` HTTP dimmer verification (`SHDM-1`/`SHDM-2`, Gen2/Gen3 dimmer `app`/`model`) and mDNS name/TXT fallback.
  - `client.go`: Gen1 `/light/0` & Gen2 `/rpc/Light.Set` HTTP client.

## Out of Scope

- Shelly HTTP basic auth.

## Open Questions

- None.
