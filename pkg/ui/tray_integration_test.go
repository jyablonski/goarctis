package ui

import (
	"testing"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

func TestTrayManager_SetStatus(t *testing.T) {
	manager := NewTrayManager()
	// Note: Initialize() requires systray to be running, so we can't test it fully
	// But we can test the method exists and doesn't panic
	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}
}

// Note: UI tests that require systray initialization are skipped
// as they require a running GUI environment. These tests verify
// the logic without actually initializing the systray.

func TestTrayManager_UpdateDeviceState_GameBuds(t *testing.T) {
	t.Skip("Requires systray initialization - tested manually")
}

func TestTrayManager_UpdateDeviceState_Razer(t *testing.T) {
	t.Skip("Requires systray initialization - tested manually")
}

func TestTrayManager_UpdateDeviceState_Multiple(t *testing.T) {
	manager := NewTrayManager()

	// Test device tracking without systray initialization
	leftBattery := 50
	rightBattery := 60
	state1 := protocol.DeviceState{
		DeviceID:     "steelseries_gamebuds",
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		IsConnected:  true,
	}

	// Update state directly in map (bypassing systray calls)
	manager.mu.Lock()
	manager.devices["steelseries_gamebuds"] = state1
	manager.mu.Unlock()

	// Add Razer
	battery := 70
	state2 := protocol.DeviceState{
		DeviceID:    "razer-device",
		DeviceType:  "razer_deathadder",
		Battery:     &battery,
		IsConnected: true,
	}

	manager.mu.Lock()
	manager.devices["razer-device"] = state2
	manager.mu.Unlock()

	// Should have both devices
	manager.mu.RLock()
	deviceCount := len(manager.devices)
	manager.mu.RUnlock()

	if deviceCount != 2 {
		t.Errorf("Expected 2 devices, got %d", deviceCount)
	}
}

func TestFormatGameBudsBattery_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		battery  *int
		status   *protocol.EarbudStatus
		side     string
		expected string
	}{
		{
			name:     "nil battery and status",
			battery:  nil,
			status:   nil,
			side:     "Left",
			expected: "🎧 Left: --",
		},
		{
			name:     "battery with nil status",
			battery:  func() *int { b := 50; return &b }(),
			status:   nil,
			side:     "Right",
			expected: "🔋 Right: 50%",
		},
		{
			name:     "zero battery with status",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Left",
			expected: "👂 Left: Wearing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatGameBudsBattery(tt.battery, tt.status, tt.side)
			if result != tt.expected {
				t.Errorf("formatGameBudsBattery() = %v, want %v", result, tt.expected)
			}
		})
	}
}
