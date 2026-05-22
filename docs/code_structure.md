# Project Structure

This document describes the organization of the goarctis project.

## Directory Structure

```
goarctis/
├── assets/                   # Static and embedded visual assets
│   ├── assets.go             # Embeds tray icon bytes for systray
│   ├── goarctis.svg          # README/project logo
│   ├── tray-screenshot.png   # README screenshot
│   └── png/
│       └── goarctis-tray-48.png
│
├── cmd/                      # Application entry points
│   ├── goarctis/
│   │   └── main.go          # Main application
│   └── test-razer/
│       └── main.go          # Razer device discovery test utility
│
├── pkg/                      # Reusable packages
│   ├── device/              # Device abstraction and implementations
│   │   ├── interface.go     # BatteryDevice interface
│   │   ├── manager.go       # Multi-device coordination
│   │   ├── hidraw.go        # SteelSeries GameBuds implementation
│   │   ├── hyperx.go        # HyperX Cloud Alpha Wireless implementation
│   │   ├── openrazer.go     # Razer devices implementation and HID warning fallback
│   │   ├── razer_hid_fallback.go # Razer sysfs HID warning fallback
│   │   └── *_test.go        # Test files
│   │
│   ├── docker/              # Docker container monitoring
│   │   ├── monitor.go       # Docker CLI polling and stop-all action
│   │   └── monitor_test.go
│   │
│   ├── protocol/            # Protocol parsing
│   │   ├── handler.go       # SteelSeries HID report parser
│   │   └── handler_test.go
│   │
│   ├── system/              # Host resource monitoring
│   │   ├── monitor.go       # CPU/memory polling and spike detection
│   │   ├── proc.go          # Linux procfs parsing
│   │   └── *_test.go
│   │
│   ├── selfupdate/          # GitHub release self-update flow
│   │   ├── selfupdate.go
│   │   └── selfupdate_test.go
│   │
│   ├── ui/                   # User interface
│   │   ├── tray.go          # System tray implementation
│   │   └── tray_test.go
│
│   └── version/              # Build-time version injection
│       └── version.go
│
├── docs/                     # Documentation
│   ├── code_structure.md    # This file
│   └── how_it_works.md
│
├── scripts/                  # Utility scripts
│   └── update_systemd.sh    # Systemd service setup
│
├── bin/                      # Build artifacts (gitignored)
│
├── README.md                 # Project overview and quick start
├── LICENSE                   # License file
├── Makefile                  # Build automation
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
└── .gitignore               # Git ignore rules
```

## Package Organization

### `cmd/` - Application Entry Points

- **goarctis/**: Main application that coordinates all components
- **test-razer/**: Standalone utility for testing Razer device discovery

### `assets/` - Visual Assets

- **assets.go**: Embeds the tray PNG used by `systray.SetIcon`
- **goarctis.svg**: Source logo used by the README
- **png/goarctis-tray-48.png**: Tray-specific PNG with tight padding for AppIndicator panels

### `pkg/device/` - Device Abstraction

- **interface.go**: Defines the `BatteryDevice` interface that all device implementations must satisfy
- **manager.go**: `DeviceManager` coordinates discovery and lifecycle of multiple devices
- **hidraw.go**: SteelSeries GameBuds implementation using HID raw device access
- **hyperx.go**: HyperX Cloud Alpha Wireless implementation using hidraw request/response polling
- **openrazer.go**: Razer devices implementation using OpenRazer D-Bus
- **razer_hid_fallback.go**: Sysfs HID fallback warning detection when OpenRazer stops reporting battery data

### `pkg/docker/` - Docker Monitoring

- **monitor.go**: Polls `docker ps --format` for running container state and exposes a stop-all helper

### `pkg/protocol/` - Protocol Parsing

- **handler.go**: Parses SteelSeries-specific HID reports into structured `DeviceState`

### `pkg/system/` - Host Resource Monitoring

- **proc.go**: Reads `/proc/stat` and `/proc/meminfo` for CPU and memory samples
- **monitor.go**: Tracks current CPU, recent CPU peak, spike hold state, and memory utilization

### `pkg/ui/` - User Interface

- **tray.go**: System tray implementation using systray library

### `pkg/selfupdate/` and `pkg/version/` - Support Packages

- **selfupdate.go**: Updates the installed binary from GitHub release assets
- **version.go**: Holds the build-time version string set by Makefile ldflags

## Design Principles

1. **Separation of Concerns**: Each package has a clear responsibility
2. **Interface-Based Design**: Device implementations use the `BatteryDevice` interface
3. **Testability**: Each package has corresponding test files
4. **Extensibility**: New device types can be added by implementing `BatteryDevice`

## Adding New Devices

To add support for a new device type:

1. Implement the `BatteryDevice` interface in `pkg/device/`
2. Add device discovery logic to `DeviceManager.DiscoverDevices()`
3. Update UI in `pkg/ui/tray.go` to handle the new device type
4. Add tests for the new implementation
