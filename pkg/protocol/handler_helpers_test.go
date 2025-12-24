package protocol

import (
	"testing"
)

func TestGetPrimaryBattery(t *testing.T) {
	tests := []struct {
		name     string
		state    DeviceState
		expected int
	}{
		{
			name: "Single battery",
			state: DeviceState{
				Battery: func() *int { b := 75; return &b }(),
			},
			expected: 75,
		},
		{
			name: "Dual battery - left lower",
			state: DeviceState{
				LeftBattery:  func() *int { b := 50; return &b }(),
				RightBattery: func() *int { b := 80; return &b }(),
			},
			expected: 50,
		},
		{
			name: "Dual battery - right lower",
			state: DeviceState{
				LeftBattery:  func() *int { b := 80; return &b }(),
				RightBattery: func() *int { b := 50; return &b }(),
			},
			expected: 50,
		},
		{
			name: "Left battery only",
			state: DeviceState{
				LeftBattery: func() *int { b := 60; return &b }(),
			},
			expected: 60,
		},
		{
			name: "Right battery only",
			state: DeviceState{
				RightBattery: func() *int { b := 70; return &b }(),
			},
			expected: 70,
		},
		{
			name:     "No battery",
			state:    DeviceState{},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.GetPrimaryBattery()
			if result != tt.expected {
				t.Errorf("GetPrimaryBattery() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestParseInEarEvent(t *testing.T) {
	h := NewHandler()
	callbackCalled := false

	h.SetOnChange(func(state DeviceState) {
		callbackCalled = true
	})

	// Test earbud removed
	err := h.ParseReport([]byte{ReportInEarEvent, 1})
	if err != nil {
		t.Errorf("ParseReport for InEarEvent returned error: %v", err)
	}

	// Test earbud placed
	err = h.ParseReport([]byte{ReportInEarEvent, 0})
	if err != nil {
		t.Errorf("ParseReport for InEarEvent returned error: %v", err)
	}

	// InEarEvent doesn't change state, so callback shouldn't be called
	if callbackCalled {
		t.Error("InEarEvent should not trigger state change callback")
	}
}

func TestEncodeCommand(t *testing.T) {
	h := NewHandler()

	_, err := h.EncodeCommand("test", "param")
	if err == nil {
		t.Error("EncodeCommand should return error (not yet implemented)")
	}
	if err.Error() != "command encoding not yet implemented" {
		t.Errorf("Expected 'command encoding not yet implemented', got: %v", err)
	}
}
