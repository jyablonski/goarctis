package ui

import (
	"testing"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

func TestGetBatteryIcon(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		expected string
	}{
		{"Full battery", 100, "🔋"},
		{"High battery", 85, "🔋"},
		{"Medium-high battery", 80, "🔋"},
		{"Medium battery", 60, "🔋"},
		{"Medium-low battery", 50, "🔋"},
		{"Low battery", 30, "🪫"},
		{"Very low battery", 20, "🪫"},
		{"Critical battery", 10, "🪫"},
		{"Almost dead", 1, "🪫"},
		{"Unknown/zero", 0, "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBatteryIcon(tt.level)
			if got != tt.expected {
				t.Errorf("getBatteryIcon(%d) = %v, want %v", tt.level, got, tt.expected)
			}
		})
	}
}

func TestGetANCIcon(t *testing.T) {
	tests := []struct {
		name     string
		mode     protocol.ANCMode
		expected string
	}{
		{"ANC Active", protocol.ANCActive, "🔇"},
		{"ANC Transparency", protocol.ANCTransparency, "👂"},
		{"ANC Off", protocol.ANCOff, "🔊"},
		{"Unknown mode", protocol.ANCMode(99), "🎧"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getANCIcon(tt.mode)
			if got != tt.expected {
				t.Errorf("getANCIcon(%v) = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

func TestFormatGameBudsBattery(t *testing.T) {
	tests := []struct {
		name     string
		battery  *int
		status   *protocol.EarbudStatus
		side     string
		expected string
	}{
		{
			name:     "In case with battery",
			battery:  func() *int { b := 80; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusInCase; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 80% - Charging",
		},
		{
			name:     "In case without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusInCase; return &s }(),
			side:     "Right",
			expected: "📦 Right: In Case",
		},
		{
			name:     "Out of case with battery",
			battery:  func() *int { b := 75; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusOut; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 75% - Out",
		},
		{
			name:     "Out of case without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusOut; return &s }(),
			side:     "Right",
			expected: "🎧 Right: Out",
		},
		{
			name:     "Wearing with high battery",
			battery:  func() *int { b := 90; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 90% - Wearing",
		},
		{
			name:     "Wearing with low battery",
			battery:  func() *int { b := 15; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Right",
			expected: "🪫 Right: 15% - Wearing",
		},
		{
			name:     "Wearing without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Left",
			expected: "👂 Left: Wearing",
		},
		{
			name:     "Unknown status with battery",
			battery:  func() *int { b := 50; return &b }(),
			status:   nil,
			side:     "Left",
			expected: "🔋 Left: 50%",
		},
		{
			name:     "Unknown status without battery",
			battery:  nil,
			status:   nil,
			side:     "Right",
			expected: "🎧 Right: --",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatGameBudsBattery(tt.battery, tt.status, tt.side)
			if got != tt.expected {
				t.Errorf("formatGameBudsBattery(%v, %v, %s) = %v, want %v",
					tt.battery, tt.status, tt.side, got, tt.expected)
			}
		})
	}
}

func TestNewTrayManager(t *testing.T) {
	manager := NewTrayManager()

	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}

	if manager.mStatus != nil {
		t.Error("mStatus should be nil before Initialize")
	}
	if manager.gameBudsMenu != nil {
		t.Error("gameBudsMenu should be nil before Initialize")
	}
	if manager.razerMenu != nil {
		t.Error("razerMenu should be nil before Initialize")
	}
	if manager.mQuit != nil {
		t.Error("mQuit should be nil before Initialize")
	}
}

func TestGameBudsTrayBattery(t *testing.T) {
	tests := []struct {
		name     string
		state    protocol.DeviceState
		expected int
	}{
		{
			name: "Both wearing, left lower",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(50),
				RightBattery: intPtr(75),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both wearing, right lower",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(80),
				RightBattery: intPtr(60),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 60,
		},
		{
			name: "Left in case, right wearing",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(90),
				RightBattery: intPtr(50),
				LeftStatus:   earbudStatusPtr(protocol.StatusInCase),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both in case falls back to available batteries",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(80),
				RightBattery: intPtr(75),
				LeftStatus:   earbudStatusPtr(protocol.StatusInCase),
				RightStatus:  earbudStatusPtr(protocol.StatusInCase),
			},
			expected: 75,
		},
		{
			name: "One out with zero battery",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(0),
				RightBattery: intPtr(50),
				LeftStatus:   earbudStatusPtr(protocol.StatusOut),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both zero battery",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(0),
				RightBattery: intPtr(0),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gameBudsTrayBattery(tt.state)
			if got != tt.expected {
				t.Errorf("gameBudsTrayBattery() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestSingleBatteryFormatting(t *testing.T) {
	if got := formatBatteryMenuTitle(intPtr(72)); got != "  🔋 Battery: 72%" {
		t.Errorf("formatBatteryMenuTitle() = %q", got)
	}
	if got := formatBatteryMenuTitle(nil); got != "  Battery: --" {
		t.Errorf("formatBatteryMenuTitle(nil) = %q", got)
	}

	charging := true
	state := protocol.DeviceState{IsCharging: &charging}
	if got := formatHyperXBatteryMenuTitle(state); got != "  ⚡ Battery: Charging" {
		t.Errorf("formatHyperXBatteryMenuTitle(charging) = %q", got)
	}

	if got := formatTrayBattery("🎧", 45, false); got != "🎧 45%" {
		t.Errorf("formatTrayBattery(with battery) = %q", got)
	}
	if got := formatTrayBattery("🎧", -1, true); got != "🎧 ⚡" {
		t.Errorf("formatTrayBattery(charging) = %q", got)
	}
	if got := formatTrayBattery("🎧", -1, false); got != "🎧 --" {
		t.Errorf("formatTrayBattery(empty) = %q", got)
	}
}

func BenchmarkFormatGameBudsBattery(b *testing.B) {
	battery := 75
	status := protocol.StatusWorn
	for i := 0; i < b.N; i++ {
		formatGameBudsBattery(&battery, &status, "Left")
	}
}

func BenchmarkGetBatteryIcon(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getBatteryIcon(75)
	}
}

func BenchmarkGetANCIcon(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getANCIcon(protocol.ANCActive)
	}
}

func TestJoinNames(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		max      int
		expected string
	}{
		{
			name:     "Empty list",
			names:    []string{},
			max:      3,
			expected: "",
		},
		{
			name:     "Single name",
			names:    []string{"web"},
			max:      3,
			expected: "web",
		},
		{
			name:     "Under limit",
			names:    []string{"web", "db"},
			max:      3,
			expected: "web, db",
		},
		{
			name:     "At limit",
			names:    []string{"web", "db", "redis"},
			max:      3,
			expected: "web, db, redis",
		},
		{
			name:     "Over limit",
			names:    []string{"web", "db", "redis", "worker", "scheduler"},
			max:      3,
			expected: "web, db, redis +2 more",
		},
		{
			name:     "One over limit",
			names:    []string{"web", "db", "redis", "worker"},
			max:      3,
			expected: "web, db, redis +1 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinNames(tt.names, tt.max)
			if got != tt.expected {
				t.Errorf("joinNames(%v, %d) = %q, want %q", tt.names, tt.max, got, tt.expected)
			}
		})
	}
}

func TestNewTrayManager_DockerFields(t *testing.T) {
	manager := NewTrayManager()

	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}

	if manager.dockerMenu != nil {
		t.Error("dockerMenu should be nil before Initialize")
	}
	if manager.dockerStatus != nil {
		t.Error("dockerStatus should be nil before Initialize")
	}
	if manager.dockerStopAll != nil {
		t.Error("dockerStopAll should be nil before Initialize")
	}
}

func TestTrayConfig_Defaults(t *testing.T) {
	cfg := TrayConfig{}

	if cfg.DisableGameBuds {
		t.Error("DisableGameBuds should default to false")
	}
	if cfg.DisableRazer {
		t.Error("DisableRazer should default to false")
	}
	if cfg.DisableHyperX {
		t.Error("DisableHyperX should default to false")
	}
}

func TestTrayManager_DisabledGameBuds_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	leftBattery := 80
	rightBattery := 70
	state := protocol.DeviceState{
		DeviceID:     "steelseries_gamebuds",
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		IsConnected:  true,
	}

	manager.updateGameBuds(state)

	if manager.gameBudsMenu != nil {
		t.Error("gameBudsMenu should remain nil when disabled")
	}
}

func TestTrayManager_DisabledRazer_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	battery := 70
	state := protocol.DeviceState{
		DeviceID:    "razer-device",
		DeviceType:  "razer_deathadder",
		Battery:     &battery,
		IsConnected: true,
	}

	manager.updateRazer(state)

	if manager.razerMenu != nil {
		t.Error("razerMenu should remain nil when disabled")
	}
}

func TestTrayManager_DisabledHyperX_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	battery := 70
	state := protocol.DeviceState{
		DeviceID:    "hyperx_cloud_alpha_wireless",
		DeviceType:  protocol.DeviceTypeHyperXCloudAlpha,
		Battery:     &battery,
		IsConnected: true,
	}

	manager.updateHyperX(state)

	if manager.hyperxMenu != nil {
		t.Error("hyperxMenu should remain nil when disabled")
	}
}

func intPtr(v int) *int {
	return &v
}

func earbudStatusPtr(v protocol.EarbudStatus) *protocol.EarbudStatus {
	return &v
}
