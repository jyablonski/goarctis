package protocol

import (
	"fmt"
	"log"
)

const (
	// Report IDs
	ReportBattery    = 0xB7
	ReportWearStatus = 0xB5
	ReportANCMode    = 0xBD
	ReportInEarEvent = 0xC6
)

// ANCMode represents the noise cancellation mode
type ANCMode int

const (
	ANCOff ANCMode = iota
	ANCTransparency
	ANCActive
)

func (m ANCMode) String() string {
	switch m {
	case ANCOff:
		return "Off"
	case ANCTransparency:
		return "Transparency"
	case ANCActive:
		return "Active Noise Cancellation"
	default:
		return "Unknown"
	}
}

// EarbudStatus represents where an earbud is
type EarbudStatus int

const (
	StatusInCase EarbudStatus = 1
	StatusOut    EarbudStatus = 2
	StatusWorn   EarbudStatus = 3
)

func (s EarbudStatus) String() string {
	switch s {
	case StatusInCase:
		return "In Case"
	case StatusOut:
		return "Out of Case"
	case StatusWorn:
		return "Wearing"
	default:
		return "Unknown"
	}
}

// DeviceState represents the current state of a device
// Fields are optional - only populated for devices that support them
type DeviceState struct {
	DeviceID        string
	DeviceType      string
	Battery         *int          // Primary battery level (for single-battery devices)
	LeftBattery     *int          // Left battery (for dual-battery devices like GameBuds)
	RightBattery    *int          // Right battery (for dual-battery devices like GameBuds)
	DockBattery     *int          // Dock/case battery (for GameBuds)
	IsCharging      *bool         // Whether device is charging
	LeftStatus      *EarbudStatus // Left earbud status (GameBuds only)
	RightStatus     *EarbudStatus // Right earbud status (GameBuds only)
	ANCMode         *ANCMode      // ANC mode (GameBuds only)
	IsConnected     bool
	FirmwareVersion string
}

// GetPrimaryBattery returns the primary battery level
// For dual-battery devices, returns the lower of the two
// Returns -1 if no battery information is available
func (s DeviceState) GetPrimaryBattery() int {
	if s.Battery != nil {
		return *s.Battery
	}
	if s.LeftBattery != nil && s.RightBattery != nil {
		left := *s.LeftBattery
		right := *s.RightBattery
		if left < right {
			return left
		}
		return right
	}
	if s.LeftBattery != nil {
		return *s.LeftBattery
	}
	if s.RightBattery != nil {
		return *s.RightBattery
	}
	return -1
}

func (s DeviceState) String() string {
	switch s.DeviceType {
	case "steelseries_gamebuds":
		leftStr := "--"
		rightStr := "--"
		ancStr := "Unknown"

		if s.LeftBattery != nil {
			leftStatus := "Unknown"
			if s.LeftStatus != nil {
				leftStatus = s.LeftStatus.String()
			}
			leftStr = fmt.Sprintf("%d%% (%s)", *s.LeftBattery, leftStatus)
		}
		if s.RightBattery != nil {
			rightStatus := "Unknown"
			if s.RightStatus != nil {
				rightStatus = s.RightStatus.String()
			}
			rightStr = fmt.Sprintf("%d%% (%s)", *s.RightBattery, rightStatus)
		}
		if s.ANCMode != nil {
			ancStr = s.ANCMode.String()
		}

		return fmt.Sprintf("L:%s | R:%s | ANC:%s", leftStr, rightStr, ancStr)
	case "razer_deathadder":
		batteryStr := "--"
		chargingStr := ""
		if s.Battery != nil {
			batteryStr = fmt.Sprintf("%d%%", *s.Battery)
		}
		if s.IsCharging != nil && *s.IsCharging {
			chargingStr = " (Charging)"
		}
		return fmt.Sprintf("Battery: %s%s", batteryStr, chargingStr)
	default:
		batteryStr := "--"
		if s.Battery != nil {
			batteryStr = fmt.Sprintf("%d%%", *s.Battery)
		}
		return fmt.Sprintf("Battery: %s", batteryStr)
	}
}

// Handler processes HID reports from the GameBuds
type Handler struct {
	state    DeviceState
	onChange func(DeviceState) // Callback when state changes
}

func NewHandler() *Handler {
	return &Handler{
		state: DeviceState{
			DeviceType:  "steelseries_gamebuds",
			IsConnected: true,
		},
	}
}

// SetOnChange sets a callback for when device state changes
func (h *Handler) SetOnChange(callback func(DeviceState)) {
	h.onChange = callback
}

// ParseReport parses incoming HID data
func (h *Handler) ParseReport(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty report")
	}

	reportID := data[0]
	oldState := h.copyState()

	switch reportID {
	case ReportBattery:
		h.parseBattery(data)
	case ReportWearStatus:
		h.parseWearStatus(data)
	case ReportANCMode:
		h.parseANCMode(data)
	case ReportInEarEvent:
		h.parseInEarEvent(data)
	default:
		// Unknown report, log for discovery
		log.Printf("Unknown report 0x%02X: %x", reportID, data)
		return nil
	}

	// If state changed, trigger callback
	if h.onChange != nil && !h.statesEqual(oldState, h.state) {
		h.onChange(h.state)
	}

	return nil
}

// copyState creates a deep copy of the state for comparison
func (h *Handler) copyState() DeviceState {
	state := h.state
	copy := DeviceState{
		DeviceID:        state.DeviceID,
		DeviceType:      state.DeviceType,
		IsConnected:     state.IsConnected,
		FirmwareVersion: state.FirmwareVersion,
	}
	if state.Battery != nil {
		b := *state.Battery
		copy.Battery = &b
	}
	if state.LeftBattery != nil {
		b := *state.LeftBattery
		copy.LeftBattery = &b
	}
	if state.RightBattery != nil {
		b := *state.RightBattery
		copy.RightBattery = &b
	}
	if state.DockBattery != nil {
		b := *state.DockBattery
		copy.DockBattery = &b
	}
	if state.IsCharging != nil {
		b := *state.IsCharging
		copy.IsCharging = &b
	}
	if state.LeftStatus != nil {
		s := *state.LeftStatus
		copy.LeftStatus = &s
	}
	if state.RightStatus != nil {
		s := *state.RightStatus
		copy.RightStatus = &s
	}
	if state.ANCMode != nil {
		m := *state.ANCMode
		copy.ANCMode = &m
	}
	return copy
}

// statesEqual compares two DeviceState structs, handling pointer fields
func (h *Handler) statesEqual(s1, s2 DeviceState) bool {
	if s1.DeviceID != s2.DeviceID || s1.DeviceType != s2.DeviceType || s1.IsConnected != s2.IsConnected {
		return false
	}
	if !pointerEqual(s1.Battery, s2.Battery) ||
		!pointerEqual(s1.LeftBattery, s2.LeftBattery) ||
		!pointerEqual(s1.RightBattery, s2.RightBattery) ||
		!pointerEqual(s1.DockBattery, s2.DockBattery) ||
		!pointerEqual(s1.IsCharging, s2.IsCharging) {
		return false
	}
	if !pointerEqual(s1.LeftStatus, s2.LeftStatus) ||
		!pointerEqual(s1.RightStatus, s2.RightStatus) ||
		!pointerEqual(s1.ANCMode, s2.ANCMode) {
		return false
	}
	return true
}

// pointerEqual compares two pointers of the same type
func pointerEqual[T comparable](p1, p2 *T) bool {
	if p1 == nil && p2 == nil {
		return true
	}
	if p1 == nil || p2 == nil {
		return false
	}
	return *p1 == *p2
}

func (h *Handler) parseBattery(data []byte) {
	if len(data) < 3 {
		return
	}

	leftBattery := int(data[1])
	rightBattery := int(data[2])

	// Only update battery if it's a valid reading (not 0 unless actually dead)
	// When an earbud is in the case, the device sometimes reports 0
	leftStatus := StatusInCase
	if h.state.LeftStatus != nil {
		leftStatus = *h.state.LeftStatus
	}
	rightStatus := StatusInCase
	if h.state.RightStatus != nil {
		rightStatus = *h.state.RightStatus
	}

	if leftBattery > 0 || leftStatus == StatusInCase {
		h.state.LeftBattery = &leftBattery
	}
	if rightBattery > 0 || rightStatus == StatusInCase {
		h.state.RightBattery = &rightBattery
	}

	log.Printf("🔋 Battery: Left %d%%, Right %d%%", leftBattery, rightBattery)
}

func (h *Handler) parseWearStatus(data []byte) {
	if len(data) < 5 {
		return
	}

	leftStatus := EarbudStatus(data[3])
	rightStatus := EarbudStatus(data[4])
	h.state.LeftStatus = &leftStatus
	h.state.RightStatus = &rightStatus

	log.Printf("👂 Status: Left=%s, Right=%s", leftStatus, rightStatus)
}

func (h *Handler) parseANCMode(data []byte) {
	if len(data) < 2 {
		return
	}

	ancMode := ANCMode(data[1])
	h.state.ANCMode = &ancMode
	log.Printf("🎧 ANC Mode: %s", ancMode)
}

func (h *Handler) parseInEarEvent(data []byte) {
	if len(data) < 2 {
		return
	}

	event := data[1]
	if event == 1 {
		log.Println("👂 Earbud removed from ear")
	} else if event == 0 {
		log.Println("👂 Earbud placed in ear")
	}
}

// GetState returns the current device state
func (h *Handler) GetState() DeviceState {
	return h.state
}

// EncodeCommand creates HID command bytes (placeholder for future)
func (h *Handler) EncodeCommand(command string, params ...interface{}) ([]byte, error) {
	// TODO: Implement when we figure out how to send commands
	return nil, fmt.Errorf("command encoding not yet implemented")
}
