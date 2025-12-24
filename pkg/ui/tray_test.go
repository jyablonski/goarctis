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

	// All menu items should be nil until Initialize is called
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

// Test battery level calculation logic
func TestLowestBatteryCalculation(t *testing.T) {
	tests := []struct {
		name          string
		leftBattery   int
		rightBattery  int
		leftStatus    protocol.EarbudStatus
		rightStatus   protocol.EarbudStatus
		expectedValue int
		shouldShow    bool
	}{
		{
			name:          "Both wearing, left lower",
			leftBattery:   50,
			rightBattery:  75,
			leftStatus:    protocol.StatusWorn,
			rightStatus:   protocol.StatusWorn,
			expectedValue: 50,
			shouldShow:    true,
		},
		{
			name:          "Both wearing, right lower",
			leftBattery:   80,
			rightBattery:  60,
			leftStatus:    protocol.StatusWorn,
			rightStatus:   protocol.StatusWorn,
			expectedValue: 60,
			shouldShow:    true,
		},
		{
			name:          "Left in case, right wearing",
			leftBattery:   90,
			rightBattery:  50,
			leftStatus:    protocol.StatusInCase,
			rightStatus:   protocol.StatusWorn,
			expectedValue: 50,
			shouldShow:    true,
		},
		{
			name:          "Both in case",
			leftBattery:   80,
			rightBattery:  75,
			leftStatus:    protocol.StatusInCase,
			rightStatus:   protocol.StatusInCase,
			expectedValue: 100,
			shouldShow:    false,
		},
		{
			name:          "One out with zero battery",
			leftBattery:   0,
			rightBattery:  50,
			leftStatus:    protocol.StatusOut,
			rightStatus:   protocol.StatusWorn,
			expectedValue: 50,
			shouldShow:    true,
		},
		{
			name:          "Both zero battery",
			leftBattery:   0,
			rightBattery:  0,
			leftStatus:    protocol.StatusWorn,
			rightStatus:   protocol.StatusWorn,
			expectedValue: 100,
			shouldShow:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from UpdateState
			lowestBattery := 100
			if tt.leftStatus != protocol.StatusInCase && tt.leftBattery > 0 {
				lowestBattery = tt.leftBattery
			}
			if tt.rightStatus != protocol.StatusInCase && tt.rightBattery > 0 && tt.rightBattery < lowestBattery {
				lowestBattery = tt.rightBattery
			}

			shouldShow := lowestBattery < 100

			if lowestBattery != tt.expectedValue {
				t.Errorf("Lowest battery = %d, want %d", lowestBattery, tt.expectedValue)
			}

			if shouldShow != tt.shouldShow {
				t.Errorf("Should show battery = %v, want %v", shouldShow, tt.shouldShow)
			}
		})
	}
}

// Benchmark the formatting functions
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
