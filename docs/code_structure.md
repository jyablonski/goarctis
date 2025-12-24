# Project Structure

This document describes the organization of the goarctis project.

## Directory Structure

```
goarctis/
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
│   │   ├── openrazer.go     # Razer devices implementation
│   │   └── *_test.go        # Test files
│   │
│   ├── protocol/            # Protocol parsing
│   │   ├── handler.go       # SteelSeries HID report parser
│   │   └── handler_test.go
│   │
│   └── ui/                   # User interface
│       ├── tray.go          # System tray implementation
│       └── tray_test.go
│
├── docs/                     # Documentation
│   ├── TESTING.md           # Testing guide
│   ├── NOTES.md             # Development notes
│   └── STRUCTURE.md         # This file
│
├── scripts/                  # Utility scripts
│   └── update_systemd.sh    # Systemd service setup
│
├── bin/                      # Build artifacts (gitignored)
│
├── README.md                 # Project overview and quick start
├── CHANGELOG.md             # Version history
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

### `pkg/device/` - Device Abstraction

- **interface.go**: Defines the `BatteryDevice` interface that all device implementations must satisfy
- **manager.go**: `DeviceManager` coordinates discovery and lifecycle of multiple devices
- **hidraw.go**: SteelSeries GameBuds implementation using HID raw device access
- **openrazer.go**: Razer devices implementation using OpenRazer D-Bus

### `pkg/protocol/` - Protocol Parsing

- **handler.go**: Parses SteelSeries-specific HID reports into structured `DeviceState`

### `pkg/ui/` - User Interface

- **tray.go**: System tray implementation using systray library

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
