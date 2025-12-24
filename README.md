# goarctis

![CI Pipeline](https://github.com/jyablonski/goarctis/actions/workflows/ci.yaml/badge.svg)
![CD Pipeline](https://github.com/jyablonski/goarctis/actions/workflows/release.yaml/badge.svg)

Linux System Tray Application written in Go for monitoring battery levels of wireless devices, including SteelSeries Arctis GameBuds and Razer devices (via OpenRazer Linux Driver).

<img width="423" height="485" alt="goarctis system tray" src="assets/tray-screenshot.png" />

## Features

### SteelSeries Arctis GameBuds

- Real-time battery monitoring for both earbuds
- ANC mode display (Active/Transparency/Off)
- Wear detection (In Case/Out/Wearing)
- System tray integration for easy access

### Razer Devices (via OpenRazer)

- Battery level monitoring for supported Razer devices
- Charging/Wireless mode detection
- Automatic reconnection handling for mode switches
- Works with any Razer device that supports battery reporting via OpenRazer

### Multi-Device Support

- Monitor multiple devices simultaneously
- Unified system tray display showing both device battery levels
- Automatic device discovery at startup

## Requirements

- Linux with PulseAudio/PipeWire
- **For GameBuds:** SteelSeries Arctis GameBuds
- **For Razer devices:** OpenRazer daemon installed and running
- **For building from source:** Go 1.25 or later

## Installation

### Quick Install (Recommended)

Download the latest release binary from [GitHub Releases](https://github.com/jyablonski/goarctis/releases):

```bash
# Download the latest release (replace v0.1.0 with the latest version)
VERSION=v0.1.0
wget https://github.com/jyablonski/goarctis/releases/download/${VERSION}/goarctis-${VERSION}-linux-amd64

# Install to system
sudo mv goarctis-${VERSION}-linux-amd64 /usr/local/bin/goarctis
sudo chmod +x /usr/local/bin/goarctis

# Verify installation
goarctis --version
```

### Build from Source

For developers or if you want to build from source:

1. **Clone the repository**:

```bash
git clone https://github.com/jyablonski/goarctis.git
cd goarctis
```

2. **Build the binary**:

```bash
make build
```

3. **Install**:

```bash
sudo cp bin/goarctis /usr/local/bin/
sudo chmod +x /usr/local/bin/goarctis
```

### Development Commands

Run the Application:

```bash
make run
```

Build the binary:

```bash
make build
```

Run Tests:

```bash
make test
```

Check Version:

```bash
./bin/goarctis --version
```

## Releases

### Creating a Release

To create a new release:

1. **Create and push a git tag**:

   ```bash
   make release VERSION=v0.2.0
   ```

   This will:

   - Create an annotated git tag with the specified version
   - Push the tag to the remote repository
   - Trigger the GitHub Actions release workflow

2. **The release workflow will automatically**:
   - Build the binary with the version injected
   - Create a GitHub release with release notes
   - Upload the compiled binary as a release artifact

### Version Detection

The application automatically detects its version:

- **Release builds**: Version is injected at build time via `-ldflags`
- **Development builds**: Uses `git describe --tags --always --dirty` or defaults to "dev"
- **Manual override**: Set `VERSION` environment variable when building

The version is displayed:

- Via the `--version` command-line flag
- In the system tray tooltip

## Documentation

For more detailed documentation, see the [`docs/`](docs/) folder:

- **[Code Structure](docs/code_structure.md)**: Detailed explanation of the project structure, package organization, and design principles
- **[How It Works](docs/how_it_works.md)**: In-depth technical documentation on HID device communication, protocol parsing, and system tray integration

## Systemd Service Setup

To run goarctis as a systemd user service (automatically starts on login and runs in the background):

1. **Create the systemd service file** (if it doesn't exist):

```bash
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/goarctis.service << EOF
[Unit]
Description=goarctis Battery Monitor
After=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goarctis
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
```

2. **Install the binary**:

**Option A: Download from GitHub Release (Recommended)**

```bash
# Download latest release (replace v0.1.0 with latest version)
VERSION=v0.1.0
wget https://github.com/jyablonski/goarctis/releases/download/${VERSION}/goarctis-${VERSION}-linux-amd64
sudo mv goarctis-${VERSION}-linux-amd64 /usr/local/bin/goarctis
sudo chmod +x /usr/local/bin/goarctis
```

**Option B: Build from Source**

```bash
make build
sudo cp bin/goarctis /usr/local/bin/
sudo chmod +x /usr/local/bin/goarctis
```

3. **Enable and start the service**:

```bash
systemctl --user daemon-reload
systemctl --user enable goarctis.service
systemctl --user start goarctis.service
```

4. **Check service status**:

```bash
systemctl --user status goarctis.service
```

### Updating the Service

**Option A: Update from GitHub Release (Recommended)**

Download and install the latest release:

```bash
# Stop the service
systemctl --user stop goarctis.service

# Download latest release (replace v0.1.0 with latest version)
VERSION=v0.1.0
wget https://github.com/jyablonski/goarctis/releases/download/${VERSION}/goarctis-${VERSION}-linux-amd64
sudo mv goarctis-${VERSION}-linux-amd64 /usr/local/bin/goarctis
sudo chmod +x /usr/local/bin/goarctis

# Start the service
systemctl --user start goarctis.service
```

**Option B: Update from Source (For Developers)**

If you're building from source, use the provided script:

```bash
./scripts/update_systemd.sh
```

This script will:

- Stop the running service
- Build the new binary from source
- Install it to `/usr/local/bin/goarctis`
- Restart the service
- Show the service status

**Note**: The systemd setup has been tested on Ubuntu. For other Linux distributions, you may need to adjust paths or service configuration. Contributions for other distributions are welcome!
