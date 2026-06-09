package protocol

import (
	"errors"
	"testing"
)

func TestParseReport_EmptyReportSentinel(t *testing.T) {
	h := NewHandler()
	if err := h.ParseReport(nil); !errors.Is(err, ErrEmptyReport) {
		t.Fatalf("ParseReport(nil) error = %v, want ErrEmptyReport", err)
	}
}

func TestParseBattery(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		initialState  DeviceState
		expectedLeft  int
		expectedRight int
	}{
		{
			name:          "normal battery reading",
			data:          []byte{0xB7, 75, 80},
			initialState:  DeviceState{},
			expectedLeft:  75,
			expectedRight: 80,
		},
		{
			name: "zero battery when in case should update",
			data: []byte{0xB7, 0, 85},
			initialState: DeviceState{
				LeftStatus:  func() *EarbudStatus { s := StatusInCase; return &s }(),
				LeftBattery: func() *int { b := 50; return &b }(),
			},
			expectedLeft:  0,
			expectedRight: 85,
		},
		{
			name: "zero battery when not in case should not update",
			data: []byte{0xB7, 0, 80},
			initialState: DeviceState{
				LeftStatus:  func() *EarbudStatus { s := StatusWorn; return &s }(),
				LeftBattery: func() *int { b := 50; return &b }(),
			},
			expectedLeft:  50,
			expectedRight: 80,
		},
		{
			name:          "both earbuds low battery",
			data:          []byte{0xB7, 10, 15},
			initialState:  DeviceState{},
			expectedLeft:  10,
			expectedRight: 15,
		},
		{
			name:          "full battery",
			data:          []byte{0xB7, 100, 100},
			initialState:  DeviceState{},
			expectedLeft:  100,
			expectedRight: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.state = tt.initialState

			h.parseBattery(tt.data)

			if h.state.LeftBattery == nil || *h.state.LeftBattery != tt.expectedLeft {
				val := 0
				if h.state.LeftBattery != nil {
					val = *h.state.LeftBattery
				}
				t.Errorf("Left battery = %d, want %d", val, tt.expectedLeft)
			}
			if h.state.RightBattery == nil || *h.state.RightBattery != tt.expectedRight {
				val := 0
				if h.state.RightBattery != nil {
					val = *h.state.RightBattery
				}
				t.Errorf("Right battery = %d, want %d", val, tt.expectedRight)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	// Real frame captured from the dongle: b0 01 01 03 03 4e 50 ...
	// byte[3]/byte[4] = wear status, byte[5]/byte[6] = left/right battery.
	realFrame := []byte{
		0xB0, 0x01, 0x01, 0x03, 0x03, 78, 80, 0x0a, 0x03, 0x00, 0x00, 0x1e,
	}

	tests := []struct {
		name           string
		data           []byte
		wantLeft       int
		wantRight      int
		wantLeftStatus EarbudStatus
		wantANC        *ANCMode
		wantSet        bool
	}{
		{
			name:           "real captured frame",
			data:           realFrame,
			wantLeft:       78,
			wantRight:      80,
			wantLeftStatus: StatusWorn,
			wantANC:        func() *ANCMode { m := ANCOff; return &m }(), // byte[9] = 0x00
			wantSet:        true,
		},
		{
			name:      "out of case low battery",
			data:      []byte{0xB0, 0x00, 0x00, 0x02, 0x02, 5, 9, 0x00},
			wantLeft:  5,
			wantRight: 9,
			wantSet:   true,
		},
		{
			name:      "active noise cancellation from byte 9",
			data:      []byte{0xB0, 0x01, 0x01, 0x03, 0x03, 80, 82, 0x0a, 0x03, 0x02},
			wantLeft:  80,
			wantRight: 82,
			wantANC:   func() *ANCMode { m := ANCActive; return &m }(),
			wantSet:   true,
		},
		{
			name:      "out-of-range ANC byte leaves ANC unset",
			data:      []byte{0xB0, 0x01, 0x01, 0x03, 0x03, 80, 82, 0x0a, 0x03, 0xFF},
			wantLeft:  80,
			wantRight: 82,
			wantANC:   nil, // 0xFF is not a valid ANCMode, so it must be ignored
			wantSet:   true,
		},
		{
			name:    "short frame ignored",
			data:    []byte{0xB0, 0x01, 0x01},
			wantSet: false,
		},
		{
			name:    "0xFF battery treated as unknown",
			data:    []byte{0xB0, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0x00},
			wantSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.state = DeviceState{}

			h.parseStatus(tt.data)

			if !tt.wantSet {
				if h.state.LeftBattery != nil || h.state.RightBattery != nil {
					t.Fatalf("expected battery to stay unset, got L=%v R=%v",
						h.state.LeftBattery, h.state.RightBattery)
				}
				return
			}

			if h.state.LeftBattery == nil || *h.state.LeftBattery != tt.wantLeft {
				t.Errorf("Left battery = %v, want %d", h.state.LeftBattery, tt.wantLeft)
			}
			if h.state.RightBattery == nil || *h.state.RightBattery != tt.wantRight {
				t.Errorf("Right battery = %v, want %d", h.state.RightBattery, tt.wantRight)
			}
			if tt.wantLeftStatus != 0 {
				if h.state.LeftStatus == nil || *h.state.LeftStatus != tt.wantLeftStatus {
					t.Errorf("Left status = %v, want %v", h.state.LeftStatus, tt.wantLeftStatus)
				}
			}
			switch {
			case tt.wantANC == nil && h.state.ANCMode != nil:
				t.Errorf("ANC mode = %v, want nil", *h.state.ANCMode)
			case tt.wantANC != nil && (h.state.ANCMode == nil || *h.state.ANCMode != *tt.wantANC):
				t.Errorf("ANC mode = %v, want %v", h.state.ANCMode, *tt.wantANC)
			}
		})
	}
}

func TestParseReport_StatusRoutesToParser(t *testing.T) {
	h := NewHandler()
	h.state = DeviceState{}

	if err := h.ParseReport([]byte{0xB0, 0x01, 0x01, 0x03, 0x03, 78, 80}); err != nil {
		t.Fatalf("ParseReport(0xB0) error = %v", err)
	}
	if h.state.LeftBattery == nil || *h.state.LeftBattery != 78 {
		t.Errorf("Left battery = %v, want 78", h.state.LeftBattery)
	}
	if h.state.RightBattery == nil || *h.state.RightBattery != 80 {
		t.Errorf("Right battery = %v, want 80", h.state.RightBattery)
	}
}

func TestSetDockBattery(t *testing.T) {
	t.Run("valid reading sets dock battery and fires callback", func(t *testing.T) {
		h := NewHandler()
		var got *DeviceState
		h.SetOnChange(func(s DeviceState) { got = &s })

		h.SetDockBattery(73)

		if h.state.DockBattery == nil || *h.state.DockBattery != 73 {
			t.Fatalf("DockBattery = %v, want 73", h.state.DockBattery)
		}
		if got == nil || got.DockBattery == nil || *got.DockBattery != 73 {
			t.Fatalf("callback DockBattery = %v, want 73", got)
		}
	})

	t.Run("unchanged value does not fire callback", func(t *testing.T) {
		h := NewHandler()
		h.SetDockBattery(73)
		fired := false
		h.SetOnChange(func(DeviceState) { fired = true })
		h.SetDockBattery(73)
		if fired {
			t.Error("callback fired for unchanged dock battery")
		}
	})

	t.Run("out of range reading is ignored", func(t *testing.T) {
		h := NewHandler()
		h.SetDockBattery(255)
		if h.state.DockBattery != nil {
			t.Fatalf("DockBattery = %v, want nil", h.state.DockBattery)
		}
	})
}

func TestParseWearStatus(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectedLeft  EarbudStatus
		expectedRight EarbudStatus
	}{
		{
			name:          "both in case",
			data:          []byte{0xB5, 0x01, 0x01, 0x01, 0x01},
			expectedLeft:  StatusInCase,
			expectedRight: StatusInCase,
		},
		{
			name:          "both out of case",
			data:          []byte{0xB5, 0x01, 0x01, 0x02, 0x02},
			expectedLeft:  StatusOut,
			expectedRight: StatusOut,
		},
		{
			name:          "both wearing",
			data:          []byte{0xB5, 0x01, 0x01, 0x03, 0x03},
			expectedLeft:  StatusWorn,
			expectedRight: StatusWorn,
		},
		{
			name:          "left wearing, right in case",
			data:          []byte{0xB5, 0x01, 0x01, 0x03, 0x01},
			expectedLeft:  StatusWorn,
			expectedRight: StatusInCase,
		},
		{
			name:          "left in case, right wearing",
			data:          []byte{0xB5, 0x01, 0x01, 0x01, 0x03},
			expectedLeft:  StatusInCase,
			expectedRight: StatusWorn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.parseWearStatus(tt.data)

			if h.state.LeftStatus == nil || *h.state.LeftStatus != tt.expectedLeft {
				t.Errorf("Left status = %v, want %v", h.state.LeftStatus, tt.expectedLeft)
			}
			if h.state.RightStatus == nil || *h.state.RightStatus != tt.expectedRight {
				t.Errorf("Right status = %v, want %v", h.state.RightStatus, tt.expectedRight)
			}
		})
	}
}

func TestParseANCMode(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectedANC ANCMode
	}{
		{
			name:        "ANC off",
			data:        []byte{0xBD, 0x00},
			expectedANC: ANCOff,
		},
		{
			name:        "ANC transparency",
			data:        []byte{0xBD, 0x01},
			expectedANC: ANCTransparency,
		},
		{
			name:        "ANC active",
			data:        []byte{0xBD, 0x02},
			expectedANC: ANCActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.parseANCMode(tt.data)

			if h.state.ANCMode == nil || *h.state.ANCMode != tt.expectedANC {
				t.Errorf("ANC mode = %v, want %v", h.state.ANCMode, tt.expectedANC)
			}
		})
	}
}

func TestParseReport(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		wantErr       bool
		checkCallback bool
	}{
		{
			name:          "valid battery report",
			data:          []byte{0xB7, 75, 80},
			wantErr:       false,
			checkCallback: true,
		},
		{
			name:          "valid wear status report",
			data:          []byte{0xB5, 0x01, 0x01, 0x03, 0x03},
			wantErr:       false,
			checkCallback: true,
		},
		{
			name:          "valid ANC mode report",
			data:          []byte{0xBD, 0x02},
			wantErr:       false,
			checkCallback: true,
		},
		{
			name:          "empty data",
			data:          []byte{},
			wantErr:       true,
			checkCallback: false,
		},
		{
			name:          "unknown report ID",
			data:          []byte{0xFF, 0x01, 0x02},
			wantErr:       false,
			checkCallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			callbackCalled := false

			h.SetOnChange(func(state DeviceState) {
				callbackCalled = true
			})

			err := h.ParseReport(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReport() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.checkCallback && !callbackCalled {
				t.Error("Expected callback to be called but it wasn't")
			}
		})
	}
}

func TestStateCallback(t *testing.T) {
	h := NewHandler()
	var capturedState DeviceState
	callCount := 0

	h.SetOnChange(func(state DeviceState) {
		capturedState = state
		callCount++
	})

	if err := h.ParseReport([]byte{0xB7, 75, 80}); err != nil {
		t.Fatalf("ParseReport returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 callback, got %d", callCount)
	}
	if capturedState.LeftBattery == nil || *capturedState.LeftBattery != 75 {
		val := 0
		if capturedState.LeftBattery != nil {
			val = *capturedState.LeftBattery
		}
		t.Errorf("Expected left battery 75, got %d", val)
	}

	if err := h.ParseReport([]byte{0xB7, 75, 80}); err != nil {
		t.Fatalf("duplicate ParseReport returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected still 1 callback after duplicate, got %d", callCount)
	}

	if err := h.ParseReport([]byte{0xB7, 74, 80}); err != nil {
		t.Fatalf("changed ParseReport returned error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 callbacks after change, got %d", callCount)
	}
}

func TestEarbudStatusString(t *testing.T) {
	tests := []struct {
		status   EarbudStatus
		expected string
	}{
		{StatusInCase, "In Case"},
		{StatusOut, "Out of Case"},
		{StatusWorn, "Wearing"},
		{EarbudStatus(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("EarbudStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestANCModeString(t *testing.T) {
	tests := []struct {
		mode     ANCMode
		expected string
	}{
		{ANCOff, "Off"},
		{ANCTransparency, "Transparency"},
		{ANCActive, "Active Noise Cancellation"},
		{ANCMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("ANCMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDeviceStateString(t *testing.T) {
	leftBattery := 75
	rightBattery := 80
	leftStatus := StatusWorn
	rightStatus := StatusOut
	ancMode := ANCActive
	state := DeviceState{
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		LeftStatus:   &leftStatus,
		RightStatus:  &rightStatus,
		ANCMode:      &ancMode,
	}

	expected := "L:75% (Wearing) | R:80% (Out of Case) | ANC:Active Noise Cancellation"
	if got := state.String(); got != expected {
		t.Errorf("DeviceState.String() = %v, want %v", got, expected)
	}
}

func TestGetState(t *testing.T) {
	h := NewHandler()
	if err := h.ParseReport([]byte{0xB7, 75, 80}); err != nil {
		t.Fatalf("battery ParseReport returned error: %v", err)
	}
	if err := h.ParseReport([]byte{0xB5, 0x01, 0x01, 0x03, 0x02}); err != nil {
		t.Fatalf("wear status ParseReport returned error: %v", err)
	}
	if err := h.ParseReport([]byte{0xBD, 0x01}); err != nil {
		t.Fatalf("ANC ParseReport returned error: %v", err)
	}

	state := h.GetState()

	if state.LeftBattery == nil || *state.LeftBattery != 75 {
		val := 0
		if state.LeftBattery != nil {
			val = *state.LeftBattery
		}
		t.Errorf("LeftBattery = %d, want 75", val)
	}
	if state.RightBattery == nil || *state.RightBattery != 80 {
		val := 0
		if state.RightBattery != nil {
			val = *state.RightBattery
		}
		t.Errorf("RightBattery = %d, want 80", val)
	}
	if state.LeftStatus == nil || *state.LeftStatus != StatusWorn {
		t.Errorf("LeftStatus = %v, want StatusWorn", state.LeftStatus)
	}
	if state.RightStatus == nil || *state.RightStatus != StatusOut {
		t.Errorf("RightStatus = %v, want StatusOut", state.RightStatus)
	}
	if state.ANCMode == nil || *state.ANCMode != ANCTransparency {
		t.Errorf("ANCMode = %v, want ANCTransparency", state.ANCMode)
	}
}
