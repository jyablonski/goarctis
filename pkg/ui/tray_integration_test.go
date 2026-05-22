package ui

import (
	"testing"

	"github.com/jyablonski/goarctis/pkg/docker"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/system"
)

func TestTrayManager_SetStatus(t *testing.T) {
	manager := NewTrayManager()
	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}
}

func TestTrayManager_UpdateDeviceState_GameBuds(t *testing.T) {
	t.Skip("Requires systray initialization - tested manually")
}

func TestTrayManager_UpdateDeviceState_Razer(t *testing.T) {
	t.Skip("Requires systray initialization - tested manually")
}

func TestTrayManager_UpdateDeviceState_Multiple(t *testing.T) {
	manager := NewTrayManager()

	leftBattery := 50
	rightBattery := 60
	state1 := protocol.DeviceState{
		DeviceID:     "steelseries_gamebuds",
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		IsConnected:  true,
	}

	manager.mu.Lock()
	manager.devices["steelseries_gamebuds"] = state1
	manager.mu.Unlock()

	battery := 70
	state2 := protocol.DeviceState{
		DeviceID:    "razer-device",
		DeviceType:  protocol.DeviceTypeRazer,
		Battery:     &battery,
		IsConnected: true,
	}

	manager.mu.Lock()
	manager.devices["razer-device"] = state2
	manager.mu.Unlock()

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

func TestTrayManager_UpdateDockerState_WithoutInitialize(t *testing.T) {
	manager := NewTrayManager()

	state := docker.DockerState{
		Available: true,
		Containers: []docker.ContainerInfo{
			{ID: "abc123", Name: "web"},
		},
	}
	manager.UpdateDockerState(state)

	manager.mu.RLock()
	stored := manager.dockerState
	manager.mu.RUnlock()

	if !stored.Available {
		t.Error("Docker state should be available")
	}
	if stored.RunningCount() != 1 {
		t.Errorf("Docker RunningCount = %d, want 1", stored.RunningCount())
	}
}

func TestTrayManager_UpdateSystemState_WithoutInitialize(t *testing.T) {
	manager := NewTrayManager()

	state := system.State{
		Available:        true,
		CPUPercent:       intPtr(18),
		MemoryPercent:    intPtr(32),
		MemoryUsedBytes:  10 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 32 * 1024 * 1024 * 1024,
	}
	manager.UpdateSystemState(state)

	manager.mu.RLock()
	stored := manager.systemState
	manager.mu.RUnlock()

	if !stored.Available {
		t.Error("System state should be available")
	}
	if stored.CPUPercent == nil || *stored.CPUPercent != 18 {
		t.Errorf("CPUPercent = %v, want 18", stored.CPUPercent)
	}
	if stored.MemoryPercent == nil || *stored.MemoryPercent != 32 {
		t.Errorf("MemoryPercent = %v, want 32", stored.MemoryPercent)
	}
}

func TestTrayManager_DockerState_WithDevices(t *testing.T) {
	manager := NewTrayManager()

	leftBattery := 50
	rightBattery := 60
	deviceState := protocol.DeviceState{
		DeviceID:     "steelseries_gamebuds",
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		IsConnected:  true,
	}

	manager.mu.Lock()
	manager.devices["steelseries_gamebuds"] = deviceState
	manager.mu.Unlock()

	dockerState := docker.DockerState{
		Available: true,
		Containers: []docker.ContainerInfo{
			{ID: "abc123", Name: "web"},
			{ID: "def456", Name: "db"},
		},
	}
	manager.mu.Lock()
	manager.dockerState = dockerState
	manager.mu.Unlock()

	manager.mu.RLock()
	deviceCount := len(manager.devices)
	dockerCount := manager.dockerState.RunningCount()
	manager.mu.RUnlock()

	if deviceCount != 1 {
		t.Errorf("Expected 1 device, got %d", deviceCount)
	}
	if dockerCount != 2 {
		t.Errorf("Expected 2 Docker containers, got %d", dockerCount)
	}
}
