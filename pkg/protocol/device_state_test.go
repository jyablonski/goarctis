package protocol

import (
	"testing"
)

func TestDeviceState_GetPrimaryBattery_AllCases(t *testing.T) {
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
		{
			name: "Single battery takes precedence over dual",
			state: DeviceState{
				Battery:      func() *int { b := 90; return &b }(),
				LeftBattery:  func() *int { b := 50; return &b }(),
				RightBattery: func() *int { b := 60; return &b }(),
			},
			expected: 90,
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

func TestDeviceState_Equal_ComparesPointerValues(t *testing.T) {
	batteryA := 65
	batteryB := 65
	chargingA := true
	chargingB := true

	leftStatusA := StatusWorn
	leftStatusB := StatusWorn
	stateA := DeviceState{
		DeviceID:    "device-1",
		DeviceType:  DeviceTypeRazerDeathAdder,
		Battery:     &batteryA,
		IsCharging:  &chargingA,
		LeftStatus:  &leftStatusA,
		IsConnected: true,
	}
	stateB := DeviceState{
		DeviceID:    "device-1",
		DeviceType:  DeviceTypeRazerDeathAdder,
		Battery:     &batteryB,
		IsCharging:  &chargingB,
		LeftStatus:  &leftStatusB,
		IsConnected: true,
	}

	if !stateA.Equal(stateB) {
		t.Fatal("states with equal pointed-to values should be equal")
	}

	changedBattery := 64
	stateB.Battery = &changedBattery
	if stateA.Equal(stateB) {
		t.Fatal("states with different pointed-to values should not be equal")
	}
}

func TestDeviceState_String_Razer(t *testing.T) {
	battery := 65
	isCharging := true
	state := DeviceState{
		DeviceType: DeviceTypeRazerDeathAdder,
		Battery:    &battery,
		IsCharging: &isCharging,
	}

	result := state.String()
	expected := "Battery: 65% (Charging)"
	if result != expected {
		t.Errorf("String() = %q, want %q", result, expected)
	}
}

func TestDeviceState_String_Razer_NotCharging(t *testing.T) {
	battery := 65
	isCharging := false
	state := DeviceState{
		DeviceType: DeviceTypeRazerDeathAdder,
		Battery:    &battery,
		IsCharging: &isCharging,
	}

	result := state.String()
	expected := "Battery: 65%"
	if result != expected {
		t.Errorf("String() = %q, want %q", result, expected)
	}
}

func TestDeviceState_String_Default(t *testing.T) {
	battery := 50
	state := DeviceState{
		DeviceType: "unknown",
		Battery:    &battery,
	}

	result := state.String()
	expected := "Battery: 50%"
	if result != expected {
		t.Errorf("String() = %q, want %q", result, expected)
	}
}
