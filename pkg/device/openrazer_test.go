package device

import (
	"errors"
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

func TestRazerSetStateComparesPointerValues(t *testing.T) {
	initialBattery := 70
	initialCharging := false
	r := &RazerDevice{
		deviceSerial: "razer-1",
		deviceName:   "Razer Test Device",
		stopChan:     make(chan struct{}),
		state: protocol.DeviceState{
			DeviceID:    "razer-1",
			DeviceType:  string(DeviceTypeRazerDeathAdder),
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
			DeviceType:  string(DeviceTypeRazerDeathAdder),
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

func TestDeviceType_String(t *testing.T) {
	if DeviceTypeSteelSeriesGameBuds != "steelseries_gamebuds" {
		t.Errorf("DeviceTypeSteelSeriesGameBuds = %q, want 'steelseries_gamebuds'", DeviceTypeSteelSeriesGameBuds)
	}
	if DeviceTypeRazerDeathAdder != "razer_deathadder" {
		t.Errorf("DeviceTypeRazerDeathAdder = %q, want 'razer_deathadder'", DeviceTypeRazerDeathAdder)
	}
	if DeviceTypeHyperXCloudAlpha != "hyperx_cloud_alpha_wireless" {
		t.Errorf("DeviceTypeHyperXCloudAlpha = %q, want 'hyperx_cloud_alpha_wireless'", DeviceTypeHyperXCloudAlpha)
	}
}
