# AGENTS.md

Guidance for coding agents working in this repository.

## Project Overview

`goarctis` is a Linux system tray app for monitoring wireless peripheral battery state and Docker container counts. It currently supports:

- SteelSeries Arctis GameBuds via hidraw input/feature reports.
- HyperX Cloud Alpha Wireless via hidraw request/response polling.
- Razer devices via OpenRazer D-Bus.
- Docker container counts via the Docker CLI.

The app is Linux-focused. Hardware behavior is often empirical, so preserve comments that document protocol quirks, device timing, permissions, or Linux hidraw behavior.

## Main Packages

- `cmd/goarctis`: application entry point, flags, systray wiring, shutdown.
- `cmd/test-razer`: small utility for Razer discovery checks.
- `pkg/device`: device discovery, lifecycle, and hardware integrations.
- `pkg/protocol`: shared device state and GameBuds HID report parsing.
- `pkg/ui`: systray menu rendering, visibility rules, and status formatting.
- `pkg/docker`: Docker polling and stop-all action.
- `pkg/selfupdate`: GitHub release self-update flow.
- `pkg/version`: build-time version injection.

## Common Commands

- `make build`: build `bin/goarctis` with ldflags version injection.
- `make build-test`: build `bin/goarctis-test` for local manual testing.
- `make run`: run the app locally.
- `make test`: run package tests with coverage through `gotestsum`.
- `go test ./...`: acceptable fallback and CI sanity check.
- `make test-razer`: run the Razer discovery utility.
- `make deps`: download and tidy modules.

Building or running the tray app requires Linux GTK/AppIndicator development libraries. Some tests use injected fakes and should not require physical devices.

## Coding Guidelines

- Keep changes scoped to the device, protocol, UI, or support package involved.
- Prefer existing helpers and interfaces over new abstractions.
- Use `protocol.DeviceState.Equal` for state-change comparisons; do not compare `DeviceState` structs directly because optional fields are pointers.
- Avoid duplicating VID/PID hidraw scanning. Use `findHIDRawDevices`.
- Keep tray section visibility rules centralized where possible.
- Keep comments sparse. Retain comments that explain hardware quirks, protocol assumptions, Linux behavior, concurrency decisions, or safety constraints. Remove comments that only narrate obvious code.
- Do not replace structured parsing or syscall/D-Bus behavior with ad hoc string tricks unless the surrounding code already uses that pattern.
- Do not introduce non-ASCII text unless the file already uses it or the UI text intentionally needs icons.
- Keep callbacks outside mutex critical sections; copy the callback and state while locked, then invoke it after unlocking.
- Make `Start`, `Stop`, and `Close` lifecycle methods idempotent and ensure every ticker, goroutine, transport, file, and D-Bus connection has a shutdown path.
- Bound external I/O with timeouts or interruptible reads; do not add polling or shutdown paths that can block indefinitely.
- Put shell commands, HTTP calls, D-Bus operations, and hardware transports behind narrow seams when adding behavior so tests can use fakes.
- Validate protocol/report lengths and sentinel values before indexing or converting bytes; return contextual errors for malformed input.
- Wrap errors with the operation and preserve sentinel identity for `errors.Is`/`errors.As`; isolate unavoidable vendor error-string matching in helpers.
- When changing stateful or concurrent code, run the focused package tests and `go test -race` in addition to `go test ./...`.

## Device-Specific Notes

### GameBuds

- Implemented in `pkg/device/hidraw.go` and `pkg/protocol/handler.go`.
- The dongle may exist before earbuds send useful state. UI should avoid showing a stale/empty GameBuds section until real earbud data arrives.
- Some systems require opening hidraw nodes read-write for feature reports.
- Input reports may be event-driven rather than continuously polling.

### HyperX

- Implemented in `pkg/device/hyperx.go`.
- Uses a three-round HID protocol: online, charging, battery.
- Draining stale reports and retrying header mismatches are important; unsolicited input reports can arrive between request and response.
- A present dongle with headset powered off/out of range is treated as disconnected in the tray.

### Razer

- Implemented in `pkg/device/openrazer.go`.
- Uses OpenRazer D-Bus APIs and includes reconnection/restart handling for daemon or device mode transitions.
- Treat `isCharging` failures as wireless/not charging unless evidence says otherwise.

### Docker

- Implemented in `pkg/docker/monitor.go`.
- Uses `docker ps --format` for structured container data.
- Keep command execution behind `CommandRunner` so tests can fake Docker.

## Testing Expectations

- Add tests for new protocol parsing, state transitions, or device discovery.
- Use injected filesystems/transports/runners rather than requiring hardware.
- For tray changes, test formatting and state selection helpers without requiring systray initialization.
- Run `go test ./...` before handing off changes. If the sandbox blocks Go's build cache, rerun with the normal cache when allowed.

## Release And Versioning

- `pkg/version.Version` is set by Makefile ldflags.
- `make release VERSION=vX.Y.Z` creates and pushes an annotated tag.
- Self-update expects GitHub release assets named `goarctis-GOOS-GOARCH`.
