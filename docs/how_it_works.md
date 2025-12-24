## How it Works

goarctis uses different methods to communicate with HID devices depending on the device type:

### SteelSeries Arctis GameBuds - HID Raw Interface

The application communicates directly with GameBuds through Linux's HID raw (`/dev/hidraw*`) interface:

1. **Device Discovery**: On startup, the application scans `/sys/class/hidraw` to find devices matching the GameBuds vendor/product IDs (1038:230a). It reads device information from sysfs to identify the correct HID interfaces.

2. **Raw HID Reading**: Once identified, the application opens the hidraw device files (`/dev/hidraw*`) and continuously reads binary HID reports in separate goroutines. These reports contain battery levels, wear status, ANC mode, and other device state information.

3. **Protocol Parsing**: The raw HID data is parsed by a protocol handler (`pkg/protocol/handler.go`) that understands different report types:

   - **Report 0xB7**: Battery levels for left and right earbuds
   - **Report 0xB5**: Wear status (In Case/Out/Wearing) for each earbud
   - **Report 0xBD**: Active Noise Cancellation mode (Off/Transparency/Active)
   - **Report 0xC6**: In-ear detection events

4. **State Management**: As reports are parsed, the device state is updated and callbacks are triggered to notify the UI layer of changes.

### Razer Devices - D-Bus via OpenRazer

For Razer devices, the application uses D-Bus to communicate with the OpenRazer Linux driver:

1. **Device Discovery**: The application connects to the session D-Bus and queries the OpenRazer daemon (`org.razer` service) to enumerate all connected Razer devices. It then tests each device to determine if it supports battery reporting.

2. **Polling Mechanism**: Unlike GameBuds which push data via HID reports, Razer devices are polled every 5 seconds. The application calls D-Bus methods:

   - `razer.device.power.getBattery()` - Retrieves battery percentage
   - `razer.device.power.isCharging()` - Determines if device is charging or in wireless mode

3. **Reconnection Handling**: The application includes robust error handling for mode switches (wired ↔ wireless). When connection errors are detected, it automatically attempts to reconnect with exponential backoff and can even restart the OpenRazer daemon if needed.

### System Tray Display

The system tray UI (`pkg/ui/tray.go`) provides real-time visualization:

1. **Icon Updates**: The tray icon title displays battery levels using emojis and percentages (e.g., `🎧 85% 🖱️ 42%`). The icon updates in real-time as device states change.

2. **Menu Structure**: Clicking the tray icon reveals a detailed menu:

   - Device-specific sections for each connected device
   - Battery levels for individual earbuds (GameBuds)
   - Charging/wireless mode indicators
   - ANC mode display (GameBuds)

3. **State Synchronization**: The `DeviceManager` (`pkg/device/manager.go`) coordinates multiple devices and routes state change callbacks to the UI. When any device state changes, the tray icon and menu items are updated accordingly.

### Architecture Overview

The application follows a modular design with clear separation of concerns:

- **Device Layer** (`pkg/device/`): Abstracts device-specific communication (HID raw for GameBuds, D-Bus for Razer)
- **Protocol Layer** (`pkg/protocol/`): Parses device-specific data formats into a unified `DeviceState` structure
- **UI Layer** (`pkg/ui/`): Handles system tray rendering and user interaction
- **Manager Layer**: Coordinates device discovery, monitoring, and state propagation

All device communication happens in background goroutines, ensuring the UI remains responsive. The main goroutine runs the system tray event loop, while device monitoring runs concurrently.
