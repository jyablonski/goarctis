# goarctis

![CI CD Pipeline](https://github.com/jyablonski/goarctis/actions/workflows/ci_cd.yaml/badge.svg)

<p align="center">
  <img width="128" height="128" alt="goarctis logo" src="assets/goarctis.svg" />
</p>

A Linux system tray application for monitoring wireless peripheral battery levels, Docker containers, and host CPU/memory/temperature utilization.

## What It Does

`goarctis` sits in the system tray and shows wireless peripheral battery state, Docker container counts, and host CPU/memory/temperature utilization.

Supported sources:

- SteelSeries Arctis GameBuds over hidraw
- HyperX Cloud Alpha Wireless over hidraw
- Razer wireless devices through OpenRazer, with a tray warning when OpenRazer stops reporting battery data
- Docker containers through the Docker CLI
- CPU and memory through Linux procfs
- Temperature sensors through Linux hwmon sysfs
- Optional NVIDIA GPU utilization, VRAM, power, fan, clock, and temperature data through NVML when an NVIDIA GPU and driver are detected

<img width="320" height="660" alt="goarctis system tray" src="assets/tray-screenshot.png" />

## Installation

**Download the latest release:**

```bash
curl -L https://github.com/jyablonski/goarctis/releases/latest/download/goarctis-linux-amd64 -o ~/.local/bin/goarctis
chmod +x ~/.local/bin/goarctis
```

Make sure `~/.local/bin` is in your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

**Or build from source:**

```bash
git clone https://github.com/jyablonski/goarctis.git
cd goarctis
make build

# Binary is at bin/goarctis
sudo cp bin/goarctis /usr/local/bin/
```

### Build Dependencies

Building from source requires GTK3 and AppIndicator development libraries:

```bash
# Ubuntu/Debian
sudo apt install libayatana-appindicator3-dev libgtk-3-dev pkg-config

# Arch Linux
sudo pacman -S libayatana-appindicator gtk3 pkgconf
```

### Runtime Requirements

- Linux desktop environment with AppIndicator support
- **GameBuds:** SteelSeries Arctis GameBuds connected via USB dongle
- **HyperX Cloud Alpha Wireless:** connected via its 2.4 GHz USB dongle. The hidraw node is owned by `root:plugdev` — the user running goarctis must be in the `plugdev` group (`sudo usermod -aG plugdev $USER`, then log out and back in).
- **Razer devices:** [OpenRazer](https://openrazer.github.io/) daemon installed and running for battery percentages. If a Razer HID device is present but OpenRazer is unavailable or not reporting battery data, goarctis shows a tray warning instead.
- **Docker monitoring:** Docker CLI available in `PATH`
- **CPU/memory monitoring:** Linux `/proc` mounted normally
- **Temperature monitoring:** Linux `/sys/class/hwmon` mounted normally. Sensor labels come from the kernel/driver and can vary by machine.
- **NVIDIA GPU monitoring:** optional. If `libnvidia-ml.so` and at least one accessible NVIDIA GPU are present, goarctis shows richer GPU metrics. Without them, system monitoring continues with hwmon/procfs only.

## Usage

| Flag                  | Description                                              | Example                             |
| --------------------- | -------------------------------------------------------- | ----------------------------------- |
| `--version`           | Print version and exit                                   | `goarctis --version`                |
| `--self-update`       | Update to the latest release and restart the service     | `goarctis --self-update`            |
| `--disable-gamebuds`  | Skip GameBuds monitoring                                 | `goarctis --disable-gamebuds`       |
| `--disable-razer`     | Skip Razer device monitoring                             | `goarctis --disable-razer`          |
| `--disable-hyperx`    | Skip HyperX Cloud Alpha Wireless monitoring              | `goarctis --disable-hyperx`         |
| `--disable-system`    | Skip CPU, memory, and temperature monitoring             | `goarctis --disable-system`         |
| `--gpu-thermal-guard` | Auto-reduce NVIDIA GPU power limit when hot (needs root) | `sudo goarctis --gpu-thermal-guard` |

Disabled sections are completely hidden from the tray dropdown menu.

Use `goarctis --help` for all available flags.

## Systemd Service

To run goarctis as a user service that starts on login:

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

systemctl --user daemon-reload
systemctl --user enable --now goarctis.service
```

To update a systemd install manually, stop the service, replace the binary, and start it again. The helper script `./scripts/update_systemd.sh` handles that flow.

## Updating

To update goarctis to the latest release:

```bash
goarctis --self-update
```

This checks the latest GitHub release, downloads the new binary, replaces the current one in place, and runs `systemctl --user restart goarctis.service` to pick up the change. If the version is already up to date it exits early.

Skipped automatically on dev builds (no version set via ldflags).

## Development

```bash
make build          # Build binary
make run            # Run the application
make test           # Run tests
make test-coverage  # Run tests with HTML coverage report
make build-test     # Build as goarctis-test (avoids conflicts with running instance)
make clean          # Remove build artifacts
make deps           # Download and tidy dependencies
```

## License

MIT
