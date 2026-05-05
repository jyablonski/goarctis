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

### Host CPU and Memory - Linux procfs

The application reads Linux procfs directly for lightweight host resource monitoring:

1. **Memory Utilization**: The system monitor reads `/proc/meminfo` and computes used memory from `MemTotal - MemAvailable`. On older kernels without `MemAvailable`, it falls back to the usual free/buffer/cache fields.

2. **CPU Utilization**: The system monitor reads the aggregate `cpu` line from `/proc/stat`. Because those values are cumulative counters, CPU utilization is calculated from the difference between two samples.

3. **Spike Detection**: CPU samples are kept in a short rolling window. The tray can show a recent peak and hold a spike indicator briefly so short CPU bursts are visible long enough to notice.

### System Tray Display

The system tray UI (`pkg/ui/tray.go`) provides real-time visualization:

1. **Icon Updates**: The tray icon title displays battery levels, Docker counts, memory, and CPU spikes using emojis and percentages (e.g., `🎧 85% 🖱️ 42% 🧠 32%`, or `🔥 87%` during a CPU spike). The icon updates in real-time as monitored state changes.

2. **Menu Structure**: Clicking the tray icon reveals a detailed menu:

   - Device-specific sections for each connected device
   - Battery levels for individual earbuds (GameBuds)
   - Charging/wireless mode indicators
   - ANC mode display (GameBuds)
   - CPU, CPU peak, and memory utilization
   - Docker container count and stop-all action

3. **State Synchronization**: The `DeviceManager` (`pkg/device/manager.go`) coordinates multiple devices and routes state change callbacks to the UI. Docker and system monitors follow the same callback pattern with their own state types. When monitored state changes, the tray icon and menu items are updated accordingly.

### Architecture Overview

The application follows a modular design with clear separation of concerns:

- **Device Layer** (`pkg/device/`): Abstracts device-specific communication (HID raw for GameBuds, D-Bus for Razer)
- **Protocol Layer** (`pkg/protocol/`): Parses device-specific data formats into a unified `DeviceState` structure
- **System Layer** (`pkg/system/`): Reads Linux procfs and turns CPU/memory counters into tray-ready state
- **Docker Layer** (`pkg/docker/`): Polls Docker container status through the Docker CLI
- **UI Layer** (`pkg/ui/`): Handles system tray rendering and user interaction
- **Manager Layer**: Coordinates device discovery, monitoring, and state propagation

All device, Docker, and system monitoring happens in background goroutines, ensuring the UI remains responsive. The main goroutine runs the system tray event loop, while monitoring runs concurrently.
