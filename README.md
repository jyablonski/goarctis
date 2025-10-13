# goarctis

Linux System Tray Application for monitoring SteelSeries Arctis GameBuds.

## Features

- Real-time battery monitoring for both earbuds
- ANC mode display (Active/Transparency/Off)
- Wear detection (In Case/Out/Wearing)
- System tray integration

## Requirements

- Go 1.24.2 or later
- Linux with PulseAudio/PipeWire
- SteelSeries Arctis GameBuds

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/yourusername/goarctis.git
cd goarctis
```

### Commands

Running the Application:

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

## Project Structure

```plaintext
goarctis/
├── cmd/
│   └── goarctis/
│       └── main.go          # Application entry point
├── pkg/
│   ├── device/
│   │   ├── hidraw.go        # Reads raw bytes from HID device and sends to protocol handler
│   │   └── hidraw_test.go
│   ├── protocol/
│   │   ├── handler.go       # Parses raw bytes into structured data (e.g., battery level, ANC mode)
│   │   └── handler_test.go
│   └── ui/
│       ├── tray.go          # System tray UI
│       └── tray_test.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```
