## How goarctis Works

goarctis is a Linux system tray app that combines hardware battery monitors, Docker container status, and host CPU/memory/temperature metrics into one AppIndicator menu and compact tray label.

### Polling Summary

| Source                      | What is checked                                                                                                                 | Interval                                                                                                                 | Tray impact                                                           |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| SteelSeries GameBuds        | Battery, wear state, and ANC reports from hidraw feature/input reports                                                          | Event-driven blocking reads, plus active feature-report polling every 5 seconds when a writable hidraw node is available | Device section, compact battery title, and tooltip                    |
| HyperX Cloud Alpha Wireless | Online, charging, and battery HID request/response rounds                                                                       | Immediately on start, then every 5 seconds                                                                               | Device section, compact battery title, and tooltip                    |
| Razer via OpenRazer         | `getBattery()` and `isCharging()` over the OpenRazer session D-Bus API                                                          | Every 5 seconds while OpenRazer is available                                                                             | Device section, compact battery title, and tooltip                    |
| Razer HID fallback          | `/sys/bus/hid/devices` scan for Razer vendor ID when OpenRazer is unavailable or not reporting battery data                     | Discovery-time fallback only; no recurring battery poll                                                                  | Warning-only device section in the popup                              |
| Docker                      | `docker ps --format "{{.ID}}\t{{.Names}}"`                                                                                      | Immediately on start, then every 10 seconds                                                                              | Docker section, compact running-count title, and tooltip              |
| System                      | CPU from `/proc/stat`, memory from `/proc/meminfo`, temperatures from `/sys/class/hwmon`, and optional NVIDIA metrics from NVML | Immediately on start, then every 2 seconds unless `--disable-system` is set                                              | System section, compact CPU/memory/GPU/temperature title, and tooltip |

### End-to-End Flow

1. `cmd/goarctis/main.go` parses CLI flags (`--version`, `--self-update`, `--disable-gamebuds`, `--disable-razer`, `--disable-hyperx`, `--disable-system`, `--gpu-thermal-guard`). `--version` prints `pkg/version.Version` (injected by the Makefile via `-ldflags`, defaulting to `dev`) and exits. `--self-update` runs `pkg/selfupdate`, which checks the latest GitHub release, downloads the matching `goarctis-GOOS-GOARCH` asset, replaces the binary, and attempts `systemctl --user restart goarctis.service`.

2. The normal path installs SIGINT/SIGTERM handling and enters `systray.Run(onReady, onExit)`. `onReady` creates a `ui.TrayManager`, sets the static embedded Arc logo from `assets/png/goarctis-tray-48.png` as the tray icon, and initializes the menu with an empty title, a disabled status row, hidden device sections, optional system rows, Docker rows, and a Quit action. The tray icon never changes with battery state.

3. `onReady` then creates a `device.DeviceManager`, registers a single state-change callback, and starts device discovery in a goroutine so the tray event loop stays responsive. Each enabled device class discovers independently; a missing device logs but does not block other classes.

4. SteelSeries GameBuds (`1038:230a`) are found by scanning `/sys/class/hidraw` via the shared `findHIDRawDevices` helper. Nodes are opened read-write first, then read-only as a fallback, because feature reports require a writable fd on some systems. Monitoring requests initial battery/wear/ANC reports, starts one blocking read goroutine per hidraw node (GameBuds are largely event-driven, so reads may block until the user removes an earbud, places it in the case, or changes ANC), and runs a 5-second polling goroutine on the first writable node. `pkg/protocol/handler.go` parses reports into a shared `protocol.DeviceState` and uses `DeviceState.Equal` to suppress spurious change notifications.

5. HyperX Cloud Alpha Wireless (`03f0:098d`) uses raw non-blocking syscalls on its hidraw node for predictable polling. Polling runs immediately and every 5 seconds, draining stale reports and then running a three-round request/response (`0x03` online, `0x0c` charging, `0x0b` battery). Responses must match the `0x21 0xbb <subcommand>` header or they are retried. If the dongle is present but the headset is off or out of range, state is marked disconnected so the section hides; if charging without a percentage, the menu shows charging instead of a stale number.

6. Razer devices come from the session D-Bus `org.razer` OpenRazer daemon, polled every 5 seconds via `razer.device.power.getBattery()` and `razer.device.power.isCharging()`. `isCharging` errors are treated as wireless/not charging; battery errors split into reconnection (closed D-Bus) versus warning-state (other failures). Connection failures back off and eventually try `systemctl --user restart openrazer-daemon.service`, falling back to `openrazer-daemon`. When OpenRazer is unavailable, goarctis scans `/sys/bus/hid/devices` for vendor ID `1532`, deduplicates by `HID_UNIQ` or `HID_ID`, and shows those as warning-only devices that do not poll D-Bus and do not contribute to the compact tray title.

7. DeviceManager stores all discovered devices behind the `BatteryDevice` interface and routes each device callback back to `cmd/goarctis` as `(deviceID, DeviceState)`, which forwards to `TrayManager.UpdateDeviceState`. The tray stores the latest state per device ID, updates the relevant section, and rebuilds the compact title and tooltip.

8. Section visibility favors hiding stale or misleading UI over showing placeholders. GameBuds sections stay hidden until real earbud data arrives (a dongle alone is not enough). HyperX and Razer sections hide when `DeviceState.IsConnected` is false. Normal Razer sections show battery and charging/wireless mode; Razer warning states hide those rows and show only the warning row.

9. The compact tray title is intentionally terse and can include device battery percentages, Docker running count, hot temperature badges, CPU spike percent, and memory percent. Unknown values are skipped so disconnected, unsupported, or warning-only devices do not add `--` noise. The tooltip carries more detail: formatted device state strings, Docker running count, and system CPU/memory/temperature summaries.

10. The Docker monitor runs regardless of device discovery, polling every 10 seconds with `docker ps --format "{{.ID}}\t{{.Names}}"`. State stores only availability and running container IDs/names; the tray shows unavailable, no containers, or a running count plus up to three names. The "Stop All Containers" row appears only when containers are running and calls `docker ps -q` then `docker stop <ids...>` through the `CommandRunner` abstraction.

11. The system monitor runs unless `--disable-system` is set, polling every 2 seconds via `pkg/system`. Memory comes from `/proc/meminfo` as `MemTotal - MemAvailable`, with a fallback for older kernels using free/buffers/cache/reclaimable slab/shared. CPU comes from the aggregate `cpu` line in `/proc/stat` and is available after the second sample (cumulative counters). Temperatures come from Linux hwmon `temp*_input` files in `/sys/class/hwmon`; CPU and GPU max values are derived from conservative chip-name/label matching, and GPU entries are shown when drivers expose GPU-like hwmon chips such as `nvidia` or `amdgpu`. NVIDIA NVML is initialized lazily with the no-GPUs flag; if `libnvidia-ml.so`, the driver, or an accessible NVIDIA GPU is absent, the NVML provider disables itself and normal procfs/hwmon monitoring continues. When NVML is available, NVIDIA hwmon GPU rows are replaced with richer utilization, VRAM, power, fan, clock, p-state, and process data. A 60-second peak window is kept, and samples at or above 80 percent start a 10-second spike hold. Notifications are thresholded: CPU and CPU peak must move at least 5 points, memory at least 1 point, temperatures at least 1 C, or availability/spike/total-memory/sensor-shape state must change.
12. When `--gpu-thermal-guard` is set, the NVIDIA sampler also runs an opt-in GPU thermal guard (`pkg/system/governor.go`) on the live device handle each poll. At or above 85 C it reduces the card's power-management limit to 80 percent of its default (bounded by the card's reported min/max), and at or below 75 C it restores the default; the gap is hysteresis to prevent oscillation. Changing the power limit needs root — on `ERROR_NO_PERMISSION`/`ERROR_NOT_SUPPORTED` the guard logs once, disables itself, and monitoring continues unaffected. On shutdown the monitor closes the sampler (`Stop` -> `io.Closer`), which restores every clamped GPU before `nvml.Shutdown`, so quitting never leaves a card throttled (and limits are session-only regardless). It is NVIDIA-only and off by default.

12. Quit clicks arrive on `TrayManager.QuitChannel` and trigger `systray.Quit()`, which runs `onExit`. Both signal handling and `onExit` call `cleanup()`, which stops Docker and system monitors, closes all devices, and releases device resources (hidraw fds, D-Bus connections). Formatting and state-selection helpers are kept separate from AppIndicator calls so tests can exercise menu logic without initializing systray, and hardware paths use injected filesystems, transports, and command runners.

### Package Responsibilities

1. `cmd/goarctis` owns flags, startup wiring, monitor lifecycles, Docker stop-all handling, quit handling, and cleanup.

2. `assets` embeds the tray PNG used by `systray.SetIcon`.

3. `pkg/device` owns hardware discovery, device lifecycles, hidraw integration, OpenRazer D-Bus integration, and Razer HID fallback warning detection.

4. `pkg/protocol` owns the shared `DeviceState` model and GameBuds HID report parsing.

5. `pkg/docker` owns Docker CLI polling and the stop-all action. Command execution is behind `CommandRunner` for tests.

6. `pkg/system` owns Linux procfs and hwmon sampling, CPU/memory/temperature state, peak/spike decoration, and notification thresholds.

7. `pkg/ui` owns AppIndicator menu construction, section visibility rules, title/tooltip formatting, and user action channels.

8. `pkg/selfupdate` owns GitHub release lookup, version comparison, binary replacement, and systemd user-service restart.

9. `pkg/version` owns the build-time version variable injected by the Makefile.
