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

type mockRazerDevice struct {
	id        string
	name      string
	state     protocol.DeviceState
	connected bool
}

func (m *mockRazerDevice) GetID() string {
	return m.id
}

func (m *mockRazerDevice) GetName() string {
	return m.name
}

func (m *mockRazerDevice) GetType() DeviceType {
	return DeviceTypeRazerDeathAdder
}

func (m *mockRazerDevice) GetState() protocol.DeviceState {
	return m.state
}

func (m *mockRazerDevice) IsConnected() bool {
	return m.connected
}

func (m *mockRazerDevice) Start() error {
	return nil
}

func (m *mockRazerDevice) Stop() error {
	return nil
}

func (m *mockRazerDevice) Close() error {
	return nil
}

func (m *mockRazerDevice) SetOnStateChange(callback func(protocol.DeviceState)) {
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
