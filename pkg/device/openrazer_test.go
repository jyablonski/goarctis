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

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "exact match",
			s:        "connection closed by user",
			substr:   "connection closed by user",
			expected: true,
		},
		{
			name:     "contains substring",
			s:        "failed to get battery: dbus: connection closed by user",
			substr:   "connection closed by user",
			expected: true,
		},
		{
			name:     "does not contain",
			s:        "some other error",
			substr:   "connection closed",
			expected: false,
		},
		{
			name:     "empty string",
			s:        "",
			substr:   "test",
			expected: false,
		},
		{
			name:     "substring longer than string",
			s:        "short",
			substr:   "this is a very long substring",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

// Mock RazerDevice for testing interface methods
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
	// Mock implementation
}

func TestDeviceType_String(t *testing.T) {
	if DeviceTypeSteelSeriesGameBuds != "steelseries_gamebuds" {
		t.Errorf("DeviceTypeSteelSeriesGameBuds = %q, want 'steelseries_gamebuds'", DeviceTypeSteelSeriesGameBuds)
	}
	if DeviceTypeRazerDeathAdder != "razer_deathadder" {
		t.Errorf("DeviceTypeRazerDeathAdder = %q, want 'razer_deathadder'", DeviceTypeRazerDeathAdder)
	}
}
