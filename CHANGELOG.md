# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Multi-device support for monitoring multiple devices simultaneously
- Razer device support via OpenRazer D-Bus integration
- Automatic device discovery for both SteelSeries and Razer devices
- Reconnection handling for Razer devices during mode switches (wired ↔ wireless)
- Separate battery level display in system tray (GameBuds lowest + Mouse)
- Device abstraction interface (`BatteryDevice`) for extensibility
- Test utility (`cmd/test-razer`) for Razer device discovery
- Comprehensive error handling and retry logic

### Changed

- Refactored `DeviceState` to support multiple device types with optional fields
- Updated UI to show both devices in system tray menu
- Improved reconnection logic with exponential backoff
- Enhanced error detection for D-Bus connection issues

### Fixed

- Fixed state comparison with pointer fields in protocol handler
- Fixed tray icon display to show both battery levels
- Fixed GameBuds battery calculation to always show lowest value
- Fixed D-Bus session bus connection (was using system bus incorrectly)

## [1.0.0] - Initial Release

### Added

- SteelSeries Arctis GameBuds support
- Real-time battery monitoring for both earbuds
- ANC mode display
- Wear detection (In Case/Out/Wearing)
- System tray integration
