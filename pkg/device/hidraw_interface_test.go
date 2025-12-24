package device

import (
	"testing"
)

func TestHIDRawManager_InterfaceMethods(t *testing.T) {
	manager := NewHIDRawManager()

	// Test GetID
	id := manager.GetID()
	if id != "steelseries_gamebuds" {
		t.Errorf("Expected ID 'steelseries_gamebuds', got '%s'", id)
	}

	// Test GetName
	name := manager.GetName()
	if name != "SteelSeries Arctis GameBuds" {
		t.Errorf("Expected name 'SteelSeries Arctis GameBuds', got '%s'", name)
	}

	// Test GetType
	deviceType := manager.GetType()
	if deviceType != DeviceTypeSteelSeriesGameBuds {
		t.Errorf("Expected type DeviceTypeSteelSeriesGameBuds, got %v", deviceType)
	}

	// Test IsConnected (no devices)
	connected := manager.IsConnected()
	if connected {
		t.Error("Expected IsConnected to return false when no devices")
	}

	// Test GetState
	state := manager.GetState()
	if state.DeviceID != "steelseries_gamebuds" {
		t.Errorf("Expected DeviceID 'steelseries_gamebuds', got '%s'", state.DeviceID)
	}
	if state.DeviceType != string(DeviceTypeSteelSeriesGameBuds) {
		t.Errorf("Expected DeviceType 'steelseries_gamebuds', got '%s'", state.DeviceType)
	}
}

func TestHIDRawManager_RealFileSystem(t *testing.T) {
	fs := RealFileSystem{}

	// Test ReadDir (should work on real filesystem)
	_, err := fs.ReadDir("/tmp")
	if err != nil {
		t.Logf("ReadDir test skipped (expected on some systems): %v", err)
	}
}
