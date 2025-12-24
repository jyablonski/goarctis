package device

import (
	"testing"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

func TestNewDeviceManager(t *testing.T) {
	dm := NewDeviceManager()
	if dm == nil {
		t.Fatal("NewDeviceManager returned nil")
	}
	if dm.devices == nil {
		t.Error("devices map should be initialized")
	}
	if len(dm.devices) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(dm.devices))
	}
}

func TestDeviceManager_SetOnStateChange(t *testing.T) {
	dm := NewDeviceManager()
	callbackCalled := false
	var receivedID string
	var receivedState protocol.DeviceState

	dm.SetOnStateChange(func(deviceID string, state protocol.DeviceState) {
		callbackCalled = true
		receivedID = deviceID
		receivedState = state
	})

	// Trigger callback via makeStateChangeHandler
	handler := dm.makeStateChangeHandler("test-device")
	testState := protocol.DeviceState{
		DeviceID:   "test-device",
		DeviceType: "test",
	}
	handler(testState)

	if !callbackCalled {
		t.Error("Callback should have been called")
	}
	if receivedID != "test-device" {
		t.Errorf("Expected deviceID 'test-device', got '%s'", receivedID)
	}
	if receivedState.DeviceID != "test-device" {
		t.Errorf("Expected state DeviceID 'test-device', got '%s'", receivedState.DeviceID)
	}
}

func TestGetDevice(t *testing.T) {
	dm := NewDeviceManager()

	// Should return nil for non-existent device
	device := dm.GetDevice("nonexistent")
	if device != nil {
		t.Error("Expected nil for non-existent device")
	}
}

func TestGetAllDevices(t *testing.T) {
	dm := NewDeviceManager()

	devices := dm.GetAllDevices()
	if devices == nil {
		t.Error("GetAllDevices should not return nil")
	}
	if len(devices) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(devices))
	}
}

func TestGetDeviceStates(t *testing.T) {
	dm := NewDeviceManager()

	states := dm.GetDeviceStates()
	if states == nil {
		t.Error("GetDeviceStates should not return nil")
	}
	if len(states) != 0 {
		t.Errorf("Expected 0 states, got %d", len(states))
	}
}

func TestStopAll(t *testing.T) {
	dm := NewDeviceManager()

	// Should not panic with no devices
	dm.StopAll()
}

func TestCloseAll(t *testing.T) {
	dm := NewDeviceManager()

	// Should not panic with no devices
	dm.CloseAll()
	if len(dm.devices) != 0 {
		t.Error("CloseAll should clear devices map")
	}
}

func TestStartAll_NoDevices(t *testing.T) {
	dm := NewDeviceManager()

	err := dm.StartAll()
	if err != nil {
		t.Errorf("StartAll with no devices should not error, got: %v", err)
	}
}

func TestDeviceManager_GetDeviceStates_WithDevices(t *testing.T) {
	dm := NewDeviceManager()

	// Add a mock device to the manager
	mockDevice := &mockHIDDevice{
		id:   "test-device",
		name: "Test Device",
		state: protocol.DeviceState{
			DeviceID:    "test-device",
			DeviceType:  "test",
			IsConnected: true,
		},
	}

	dm.mu.Lock()
	dm.devices["test-device"] = mockDevice
	dm.mu.Unlock()

	states := dm.GetDeviceStates()
	if len(states) != 1 {
		t.Errorf("Expected 1 state, got %d", len(states))
	}
	if states["test-device"].DeviceID != "test-device" {
		t.Errorf("Expected DeviceID 'test-device', got '%s'", states["test-device"].DeviceID)
	}
}

// mockHIDDevice is a simple mock for testing
type mockHIDDevice struct {
	id        string
	name      string
	state     protocol.DeviceState
	connected bool
}

func (m *mockHIDDevice) GetID() string {
	return m.id
}

func (m *mockHIDDevice) GetName() string {
	return m.name
}

func (m *mockHIDDevice) GetType() DeviceType {
	return DeviceTypeSteelSeriesGameBuds
}

func (m *mockHIDDevice) GetState() protocol.DeviceState {
	return m.state
}

func (m *mockHIDDevice) IsConnected() bool {
	return m.connected
}

func (m *mockHIDDevice) Start() error {
	return nil
}

func (m *mockHIDDevice) Stop() error {
	return nil
}

func (m *mockHIDDevice) Close() error {
	return nil
}

func (m *mockHIDDevice) SetOnStateChange(callback func(protocol.DeviceState)) {
	// Mock implementation
}
