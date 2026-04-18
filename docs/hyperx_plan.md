# HyperX Cloud Alpha Wireless — Implementation Plan

Plan for adding HyperX Cloud Alpha Wireless battery monitoring to goarctis, alongside the existing SteelSeries GameBuds and Razer support.

## Goal

- Detect a plugged-in HyperX Cloud Alpha Wireless dongle (in addition to the existing GameBuds dongle — only one audio device is expected to be present at a time, but the code shows both if both appear).
- Display its battery level in the system tray next to the Razer mouse battery.
- Pull the battery level via the lowest-friction mechanism that works on the target machine.

## Transport decision

UPower / `/sys/class/power_supply/` do not enumerate the device on the target machine (confirmed via `upower -e` and `ls /sys/class/power_supply/`). The kernel-level sysfs path is unavailable, so we implement a raw HID driver.

USB IDs and node layout (confirmed via `lsusb` and `/sys/class/hidraw`):

| Field | Value |
| --- | --- |
| USB VID | `0x03F0` (HP, Inc — HP acquired HyperX) |
| USB PID | `0x098D` |
| hidraw node | single node (e.g. `/dev/hidraw7`), `bInterfaceNumber = 03` |
| Perms | `crw-rw---- root plugdev` |

The running user must be in the `plugdev` group. This is a documentation addition only — no shipped udev rule.

## Protocol

Derived from the HeadsetControl reference implementation (`lib/devices/hyperx_cloud_alpha_wireless.hpp` in `Sapd/HeadsetControl`). Battery is **not** a feature report — it's a request/response dance using ordinary output and input reports.

Each report is 31 bytes, first three bytes `{0x21, 0xbb, <subcommand>}`. A single battery poll is three rounds:

| Round | Subcommand | Meaning of `resp[3]` |
| --- | --- | --- |
| 1 | `0x03` online check | `0x01` → headset off / out of range; else continue |
| 2 | `0x0C` charge check | `0x01` → charging over USB-C (no battery % available); else continue |
| 3 | `0x0B` battery level | response byte is battery percent |

Each round is one `write()` followed by one `read()` with a 2000 ms timeout.

## Read-timeout mechanism

The hidraw fd is opened `O_NONBLOCK`. Each read is gated by `syscall.Poll([]PollFd{{Fd: fd, Events: POLLIN}}, 2000)`. Pure Go, matches the existing raw-syscall idiom in `pkg/device/hidraw.go`, no CGO / hidapi dependency.

## Error recovery within a poll tick

hidraw is a stream: the kernel buffers input reports and hands them to the next reader regardless of which write they "belong" to. Any desync becomes permanent until explicitly cleaned up.

All three mitigations ship from day one:

1. **Drain before sequence.** Before round 1 of every poll, read the fd non-blocking until `EAGAIN` to flush leftover reports from the previous tick or from other processes.
2. **Validate response header.** After each read, require `resp[0] == 0x21 && resp[1] == 0xbb && resp[2] == <subcommand>`. On mismatch (e.g. unsolicited button event), discard and retry the read up to ~3 times.
3. **Abort the whole sequence on any round failure.** Do not attempt partial recovery mid-tick. The next tick's drain will clean up whatever the failed sequence left behind.

Explicitly rejected: closing and reopening the fd on failure. Drain-plus-retry is sufficient; reopen introduces its own races (fd reuse, udev re-permission) and can always be added later if field experience requires it.

## State machine

The three-round protocol plus the fd-level errors yield four outcomes per poll, mapped onto the existing `protocol.DeviceState` fields:

| Outcome | Trigger | `IsConnected` | `Battery` | `IsCharging` | Tray visibility |
| --- | --- | --- | --- | --- | --- |
| Normal | round 3 returns battery byte | `true` | `&N` | `&false` | section shown |
| Charging | round 2 response[3] == 0x01 | `true` | `nil` | `&true` | section shown |
| Headset off | round 1 response[3] == 0x01 | `false` | `nil` | `nil` | section hidden |
| Dongle gone | write `ENODEV` or poll timeout exhausted | `false` | `nil` | `nil` | section hidden |

"Headset off" and "dongle unplugged" collapse to the same user-visible state: hide the HyperX menu section. No new `DeviceState` fields are introduced.

## Discovery

Startup-only, matching `pkg/device/manager.go` as it stands today. No periodic rescan, no hot-plug. If the dongle is not present when goarctis boots, the HyperX section simply never appears. If the dongle is present but the headset is off, the device is added to the manager but its section stays hidden until a poll tick returns `IsConnected = true`.

If both a GameBuds dongle and a HyperX dongle are detected at startup, both are shown. No mutual-exclusion logic — the user's "only one at a time" assumption means the case is theoretical, and writing exclusion code for a case that can't happen is waste.

## Polling cadence

5 seconds, matching GameBuds and Razer.

## Code layout

New files:

- `pkg/device/hyperx.go` — `HyperXDevice` implementing `BatteryDevice`. Contains VID/PID scan (duplicated from `hidraw.go` — two usages is not enough to justify extracting a shared helper), `O_NONBLOCK` open, drain, three-round sequence, header validation, poll loop, `DeviceState` emission.
- `pkg/device/hyperx_test.go` — mirrors `hidraw_test.go`'s mock-`FileSystem` pattern. Introduces a `hidTransport` interface around write / poll / read so the three-round logic, drain, and header validation can be unit-tested without real syscalls.

Modified files:

- `pkg/device/interface.go` — add `DeviceTypeHyperXCloudAlpha DeviceType = "hyperx_cloud_alpha_wireless"`.
- `pkg/device/manager.go` — new `DisableHyperX bool` on `DiscoveryConfig`; discover HyperX after GameBuds.
- `pkg/ui/tray.go` — new `DisableHyperX` on `TrayConfig`; new HyperX menu section (single "Battery:" row, simpler than GameBuds); case in `updateTrayIcon` that reuses the existing `🎧` slot, so a HyperX-only install renders as `🎧 NN% 🖱️ NN%` identical in shape to a GameBuds-only install. Hide-section-on-disconnect logic.
- `cmd/goarctis/main.go` — `--disable-hyperx` flag plumbed through `DiscoveryConfig` and `TrayConfig`.
- `README.md` — `plugdev` prerequisite, `--disable-hyperx` row in the flag table, "HyperX Cloud Alpha Wireless via its 2.4 GHz dongle" bullet in runtime requirements.

Tray title styling (emoji choice for the charging-with-no-level state, etc.) is deferred — iterate once the code runs.

## What we do *not* know, and how to find out later

Whether the Cloud Alpha Wireless dongle emits unsolicited input reports (button forwarding, low-battery alerts, wear-detect, firmware chatter). If it does, mitigation #2 (header validation) earns its keep. If it does not, header validation is belt-and-suspenders but harmless.

Cheap way to check once the driver runs: enable debug logging on the drain path and leave goarctis running for an hour with the dongle idle. If drain ever returns non-zero bytes, the device is emitting unsolicited reports.
