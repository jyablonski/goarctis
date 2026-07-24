package device

import (
	"errors"
	"os"
	"testing"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

func TestIsConnectionClosed(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "connection closed by user",
			err:      errors.New("dbus: connection closed by user"),
			expected: true,
		},
		{
			name:     "connection closed",
			err:      errors.New("dbus: connection closed"),
			expected: true,
		},
		{
			name:     "wrapped connection closed",
			err:      errors.New("failed to get battery: dbus: connection closed by user"),
			expected: true,
		},
		{
			name:     "EOF",
			err:      errors.New("EOF"),
			expected: true,
		},
		{
			name:     "use of closed network connection",
			err:      errors.New("use of closed network connection"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConnectionClosed(tt.err)
			if result != tt.expected {
				t.Errorf("isConnectionClosed() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsUnsupportedBatteryMethod(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "unknown method",
			err:      errors.New("org.freedesktop.DBus.Error.UnknownMethod: no such method getBattery"),
			expected: true,
		},
		{
			name:     "no such interface",
			err:      errors.New("No such interface 'razer.device.power'"),
			expected: true,
		},
		{
			name:     "driver failure",
			err:      errors.New("failed to read battery from driver"),
			expected: false,
		},
		{
			name:     "nil",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUnsupportedBatteryMethod(tt.err)
			if result != tt.expected {
				t.Errorf("isUnsupportedBatteryMethod() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSelectRazerDevice(t *testing.T) {
	tests := []struct {
		name          string
		candidates    []razerDeviceCandidate
		currentSerial string
		currentName   string
		wantSerial    string
		wantFound     bool
	}{
		{
			name:          "keeps the current OpenRazer serial when available",
			candidates:    []razerDeviceCandidate{{serial: "wired-serial", path: "/org/razer/device/wired-serial", name: "Razer DeathAdder V4 Pro"}},
			currentSerial: "wired-serial",
			currentName:   "Razer DeathAdder V4 Pro",
			wantSerial:    "wired-serial",
			wantFound:     true,
		},
		{
			name:          "matches a replacement wired interface by device name",
			candidates:    []razerDeviceCandidate{{serial: "wired-serial", path: "/org/razer/device/wired-serial", name: "Razer DeathAdder V4 Pro"}},
			currentSerial: "missing-serial",
			currentName:   "Razer DeathAdder V4 Pro",
			wantSerial:    "wired-serial",
			wantFound:     true,
		},
		{
			name: "does not guess between duplicate device names",
			candidates: []razerDeviceCandidate{
				{serial: "wired-serial", path: "/org/razer/device/wired-serial", name: "Razer DeathAdder V4 Pro"},
				{serial: "other-serial", path: "/org/razer/device/other-serial", name: "Razer DeathAdder V4 Pro"},
			},
			currentSerial: "missing-serial",
			currentName:   "Razer DeathAdder V4 Pro",
			wantFound:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := selectRazerDevice(tt.candidates, tt.currentSerial, tt.currentName)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if found && got.serial != tt.wantSerial {
				t.Fatalf("serial = %q, want %q", got.serial, tt.wantSerial)
			}
		})
	}
}

func TestRazerSetStateComparesPointerValues(t *testing.T) {
	initialBattery := 70
	initialCharging := false
	r := &RazerDevice{
		deviceSerial: "razer-1",
		deviceName:   "Razer Test Device",
		stopChan:     make(chan struct{}),
		state: protocol.DeviceState{
			DeviceID:    "razer-1",
			DeviceType:  string(DeviceTypeRazer),
			Battery:     &initialBattery,
			IsCharging:  &initialCharging,
			IsConnected: true,
		},
	}

	var callbackCount int
	r.SetOnStateChange(func(protocol.DeviceState) {
		callbackCount++
	})

	r.setState(func(state *protocol.DeviceState) {
		battery := 70
		isCharging := false
		state.Battery = &battery
		state.IsCharging = &isCharging
		state.IsConnected = true
	})
	if callbackCount != 0 {
		t.Fatalf("callback fired for equivalent values; count=%d", callbackCount)
	}

	r.setState(func(state *protocol.DeviceState) {
		battery := 69
		isCharging := false
		state.Battery = &battery
		state.IsCharging = &isCharging
		state.IsConnected = true
	})
	if callbackCount != 1 {
		t.Fatalf("callback count after changed value = %d, want 1", callbackCount)
	}
}

func TestRazerStartEmitsCurrentState(t *testing.T) {
	battery := 70
	isCharging := false
	r := &RazerDevice{
		deviceSerial: "razer-1",
		deviceName:   "Razer Test Device",
		stopChan:     make(chan struct{}),
		state: protocol.DeviceState{
			DeviceID:    "razer-1",
			DeviceType:  string(DeviceTypeRazer),
			Battery:     &battery,
			IsCharging:  &isCharging,
			IsConnected: true,
		},
	}

	var got protocol.DeviceState
	var callbackCount int
	r.SetOnStateChange(func(state protocol.DeviceState) {
		got = state
		callbackCount++
	})

	if err := r.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := r.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	if callbackCount != 1 {
		t.Fatalf("callback count = %d, want 1", callbackCount)
	}
	if got.Battery == nil || *got.Battery != 70 {
		t.Fatalf("callback battery = %v, want 70", got.Battery)
	}
	if !got.IsConnected {
		t.Fatal("callback state should be connected")
	}
}

func TestRazerSetBatteryUnavailableKeepsDeviceConnected(t *testing.T) {
	battery := 70
	isCharging := false
	r := &RazerDevice{
		deviceSerial: "razer-1",
		deviceName:   "Razer Test Device",
		stopChan:     make(chan struct{}),
		state: protocol.DeviceState{
			DeviceID:    "razer-1",
			DeviceType:  string(DeviceTypeRazer),
			Battery:     &battery,
			IsCharging:  &isCharging,
			IsConnected: true,
		},
	}

	var got protocol.DeviceState
	r.SetOnStateChange(func(state protocol.DeviceState) {
		got = state
	})

	r.setBatteryUnavailable()

	if !got.IsConnected {
		t.Fatal("battery unavailable state should keep the device connected")
	}
	if got.Battery != nil {
		t.Fatalf("Battery = %v, want nil", got.Battery)
	}
	if got.IsCharging != nil {
		t.Fatalf("IsCharging = %v, want nil", got.IsCharging)
	}
	if got.Warning != razerBatteryUnavailableWarning {
		t.Fatalf("Warning = %q, want %q", got.Warning, razerBatteryUnavailableWarning)
	}
}

func TestNewRazerWarningDevice(t *testing.T) {
	device := newRazerWarningDevice("razer-hid-1", "Razer Test Mouse")

	if device.GetID() != "razer-hid-1" {
		t.Fatalf("GetID() = %q", device.GetID())
	}
	if device.GetName() != "Razer Test Mouse" {
		t.Fatalf("GetName() = %q", device.GetName())
	}
	if err := device.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	state := device.GetState()
	if !state.IsConnected {
		t.Fatal("warning device should be connected")
	}
	if state.Warning != razerBatteryUnavailableWarning {
		t.Fatalf("Warning = %q, want %q", state.Warning, razerBatteryUnavailableWarning)
	}
}

func TestFindRazerHIDDevices(t *testing.T) {
	fs := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			razerHIDSysfsDir: {
				MockFileInfo{name: "0003:00001532:000000AA.0001"},
				MockFileInfo{name: "0003:00001532:000000AA.0002"},
				MockFileInfo{name: "0003:00001038:0000230A.0002"},
			},
		},
		files: map[string][]byte{
			"/sys/bus/hid/devices/0003:00001532:000000AA.0001/uevent": []byte("HID_ID=0003:00001532:000000AA\nHID_NAME=Razer Test Mouse\nHID_UNIQ=ABC123\n"),
			"/sys/bus/hid/devices/0003:00001532:000000AA.0002/uevent": []byte("HID_ID=0003:00001532:000000AA\nHID_NAME=Razer Test Mouse\nHID_UNIQ=ABC123\n"),
			"/sys/bus/hid/devices/0003:00001038:0000230A.0002/uevent": []byte("HID_ID=0003:00001038:0000230A\n"),
			"/sys/bus/hid/devices/0003:00001038:0000230A.0002/name":   []byte("SteelSeries Arctis GameBuds\n"),
		},
	}

	devices, err := findRazerHIDDevices(fs)
	if err != nil {
		t.Fatalf("findRazerHIDDevices returned error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}
	if devices[0].Name != "Razer Test Mouse" {
		t.Fatalf("Name = %q, want Razer Test Mouse", devices[0].Name)
	}
	if devices[0].ID != "razer-hid-ABC123" {
		t.Fatalf("ID = %q, want razer-hid-ABC123", devices[0].ID)
	}
}

func TestParseUeventValue(t *testing.T) {
	data := []byte("HID_ID=0003:00001532:000000BF\nHID_NAME=Razer DeathAdder V4 Pro\n")
	if got := parseUeventValue(data, "HID_NAME"); got != "Razer DeathAdder V4 Pro" {
		t.Fatalf("parseUeventValue() = %q", got)
	}
}

func TestDeviceType_String(t *testing.T) {
	if DeviceTypeSteelSeriesGameBuds != "steelseries_gamebuds" {
		t.Errorf("DeviceTypeSteelSeriesGameBuds = %q, want 'steelseries_gamebuds'", DeviceTypeSteelSeriesGameBuds)
	}
	if DeviceTypeRazer != "razer_deathadder" {
		t.Errorf("DeviceTypeRazer = %q, want 'razer_deathadder'", DeviceTypeRazer)
	}
	if DeviceTypeHyperXCloudAlpha != "hyperx_cloud_alpha_wireless" {
		t.Errorf("DeviceTypeHyperXCloudAlpha = %q, want 'hyperx_cloud_alpha_wireless'", DeviceTypeHyperXCloudAlpha)
	}
}
