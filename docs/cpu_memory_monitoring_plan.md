# CPU and Memory Monitoring Plan

## Goal

Add lightweight Linux CPU and memory utilization tracking to the tray app, alongside the existing peripheral battery and Docker container status.

The target user experience is:

- Always see current memory utilization somewhere in the tray menu, for example `Memory: 32%`.
- See CPU spikes without needing to open a terminal, ideally from the tray title or an obvious menu row.
- Keep the existing hardware and Docker behavior intact.
- Keep the implementation testable without requiring real hardware, Docker, or a particular live system load.

## Current App Review

`cmd/goarctis/main.go` currently wires together three concerns:

- `ui.TrayManager` owns systray menu items, tray title formatting, and click channels.
- `device.DeviceManager` discovers and starts hardware integrations, then forwards `protocol.DeviceState` changes to the tray.
- `docker.Monitor` polls the Docker CLI every 10 seconds and forwards `docker.DockerState` changes to the tray.

The hardware device manager is intentionally about battery-capable peripherals. CPU and memory are not devices in that sense, so they should not be folded into `pkg/device` or `protocol.DeviceState`.

The Docker monitor is the closest existing pattern for system resource monitoring:

- It is a standalone package.
- It polls in a goroutine.
- It owns its own state type.
- It has an injected dependency for tests.
- It calls an `onChange` callback only when state meaningfully changes.
- It is started and stopped from `main.go`.

The tray layer is already centralized enough for this feature. `TrayManager` stores device state and Docker state under a mutex, formats menu rows, and builds the compact tray title in `updateTrayIcon`.

One relevant existing issue: `UpdateDockerState` updates Docker menu rows and stores Docker state, but does not call `updateTrayIcon`. Because `updateTrayIcon` already includes Docker counts, Docker title updates can depend on a later device-state update. Adding CPU/memory should fix this shared callback path so Docker and system resource state refresh the tray title consistently.

## Implementation Options

### Option 1: Read Linux `/proc` directly

Read `/proc/stat` for CPU counters and `/proc/meminfo` for memory.

CPU usage would be computed from two samples of the aggregate `cpu` line:

- `total = user + nice + system + idle + iowait + irq + softirq + steal`
- `idle = idle + iowait`
- `usage = (deltaTotal - deltaIdle) / deltaTotal * 100`

Memory usage would use `MemAvailable` when present:

- `used = MemTotal - MemAvailable`
- `percent = used / MemTotal * 100`

Pros:

- No new dependency.
- Very low overhead.
- Works well for a Linux-only app.
- Easy to unit test with fake file contents.
- Avoids shelling out every few seconds.

Cons:

- Linux-specific, though the whole app is already Linux-focused.
- CPU requires two samples before a real percentage is available.
- Short spikes can be missed unless the poll interval is short enough.

### Option 2: Add a library such as `gopsutil`

Use a cross-platform system metrics dependency to read CPU and memory.

Pros:

- Higher-level API.
- Handles some platform-specific details.
- Could make future metrics easier.

Cons:

- New dependency for a small amount of Linux-only data.
- Still needs careful sampling semantics for CPU.
- Adds more surface area than the current codebase style usually needs.

### Option 3: Shell out to commands

Use commands such as `free`, `top`, `mpstat`, or `vmstat`.

Pros:

- Quick to prototype manually.
- Mirrors how a user might inspect the system in a terminal.

Cons:

- More fragile parsing.
- Depends on optional tools and locale/output variants.
- Higher overhead than reading `/proc`.
- Harder to test cleanly.

### Option 4: Track only Docker/container resource usage

Use `docker stats` or cgroup files to report container CPU and memory.

Pros:

- Useful if the main pain point is Docker load.

Cons:

- Does not answer "my system memory is at 32%" directly.
- More complex because container stats are per-container and can be expensive.
- Better as a later feature after host CPU/memory is in place.

## Recommendation

Use Option 1: a new `pkg/system` package that reads `/proc` directly.

This matches the app's Linux scope and keeps the feature small, dependency-free, and testable. It also follows the Docker monitor shape without mixing host metrics into the hardware device manager.

## Proposed Behavior

Add a new "System" section to the tray menu:

- `⚙️ CPU: 18%`
- `🔥 CPU Peak: 92% last 60s` when a recent spike is detected
- `🧠 Memory: 32% (10.1 / 31.2 GiB)`

Tray title behavior should stay compact:

- Always include memory as `🧠 32%`, since the user explicitly wants glanceable memory percentage.
- Include CPU as `🔥 87%` while current CPU exceeds the spike threshold or during a short hold period after a spike, so a quick spike is visible long enough to notice.
- Keep normal CPU readings in the dropdown as `⚙️ CPU: 18%`.

Emoji choices should stay consistent with the current tray style:

- `⚙️` for CPU because it reads as compute/work without implying danger.
- `🔥` only for CPU spike/peak states so spikes stand out.
- `🧠` for memory because `💾` usually reads as disk/storage rather than RAM.

Suggested defaults:

- Poll interval: 2 seconds.
- CPU spike threshold: 80%.
- CPU spike hold window: 10 seconds in the tray title.
- CPU peak window: 60 seconds in the menu.
- Memory title update threshold: 1 percentage point.
- CPU title update threshold: 5 percentage points, plus threshold crossings.

These defaults keep the tray responsive without forcing title changes on every tiny counter movement.

## Proposed Design

### `pkg/system`

Create a new package with a shape similar to `pkg/docker`.

Proposed types:

```go
type State struct {
    Available        bool
    CPUPercent       *int
    CPUPeakPercent   *int
    CPUSpiking       bool
    MemoryPercent    *int
    MemoryUsedBytes  uint64
    MemoryTotalBytes uint64
}

type Sampler interface {
    Sample() (State, error)
}

type Monitor struct {
    // same lifecycle pattern as docker.Monitor
}
```

The sampler should hide the `/proc` parsing details. The monitor should own:

- Previous CPU counters, because CPU usage requires deltas.
- Rolling CPU peak samples for the last 60 seconds.
- Change detection thresholds.
- Poll loop, stop channel, state storage, and callback.

Use an injected file reader or sampler in tests so unit tests never depend on the live host's load.

### `pkg/ui`

Extend `TrayManager` with:

- `systemMenu`
- `systemCPU`
- `systemMemory`
- `systemPeak`
- `systemState system.State`

Add:

- `UpdateSystemState(system.State)`
- Formatting helpers for percentages and bytes.
- Tests for memory formatting, CPU spike formatting, and title inclusion rules.

Also update `UpdateDockerState` to call `updateTrayIcon` after storing Docker state, so Docker, CPU, and memory all refresh the compact title through the same path.

### `cmd/goarctis`

Wire the monitor in the same style as Docker:

- Add a package-level `systemMonitor *system.Monitor`.
- Add an optional flag such as `--disable-system`.
- Start the monitor from `onReady`.
- Forward changes to `trayManager.UpdateSystemState`.
- Stop it from `cleanup`.

### Documentation

After implementation, update:

- `README.md` feature list and flags table.
- `docs/how_it_works.md` with a short system resource monitoring section.
- `docs/code_structure.md` to include `pkg/docker`, `pkg/system`, `pkg/selfupdate`, and `pkg/version`, since the current structure doc is behind the actual tree.

## Implementation Steps

1. Add `pkg/system` with `/proc` parsing helpers and focused unit tests.
2. Add `system.Monitor` lifecycle and change-detection tests.
3. Add tray menu rows, formatting helpers, and `UpdateSystemState`.
4. Make Docker and system state updates refresh the tray title consistently.
5. Wire the system monitor in `cmd/goarctis/main.go`, including cleanup and an optional disable flag.
6. Update README and docs.
7. Run `go test ./...`.

## Testing Plan

Add tests for:

- `/proc/meminfo` parsing with `MemAvailable`.
- `/proc/meminfo` fallback behavior if `MemAvailable` is absent.
- `/proc/stat` CPU delta calculation.
- First CPU sample behavior, where CPU percent is unknown until a second sample exists.
- CPU spike threshold crossing.
- Rolling CPU peak expiration.
- Monitor callback only firing on meaningful changes.
- UI formatting for unavailable system metrics.
- UI formatting for normal memory, normal CPU, and CPU spike states.
- Tray state storage without systray initialization, matching the existing Docker tests.

Manual checks after implementation:

- Start the app and verify the menu shows memory immediately.
- Generate CPU load and verify the tray title/menu show a spike.
- Stop the app and verify monitor goroutines exit cleanly.
- Run with `--disable-system` and verify the System section is hidden.

## Open Decisions

- Whether memory should always be shown in the tray title or only in the dropdown. The recommendation above is to show it in the title because the requested target is glanceable `32% memory`.
- Whether CPU should show only current usage or current plus recent peak. The recommendation is current usage in normal menu state, recent peak in the menu, and title visibility during spikes.
- Whether thresholds should be fixed constants initially or exposed as flags. The recommendation is fixed constants first, then flags later only if the defaults feel noisy in daily use.
