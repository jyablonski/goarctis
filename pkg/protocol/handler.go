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

// DeviceState represents the current state of the GameBuds
type DeviceState struct {
	LeftBattery     int
	RightBattery    int
	DockBattery     int
	LeftStatus      EarbudStatus
	RightStatus     EarbudStatus
	ANCMode         ANCMode
	IsConnected     bool
	FirmwareVersion string
}

func (s DeviceState) String() string {
	return fmt.Sprintf(
		"L:%d%% (%s) | R:%d%% (%s) | ANC:%s",
		s.LeftBattery, s.LeftStatus,
		s.RightBattery, s.RightStatus,
		s.ANCMode,
	)
}

// Handler processes HID reports from the GameBuds
type Handler struct {
	state    DeviceState
	onChange func(DeviceState) // Callback when state changes
}

func NewHandler() *Handler {
	return &Handler{
		state: DeviceState{
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
	oldState := h.state

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
	if h.onChange != nil && oldState != h.state {
		h.onChange(h.state)
	}

	return nil
}

func (h *Handler) parseBattery(data []byte) {
	if len(data) < 3 {
		return
	}

	leftBattery := int(data[1])
	rightBattery := int(data[2])

	// Only update battery if it's a valid reading (not 0 unless actually dead)
	// When an earbud is in the case, the device sometimes reports 0
	if leftBattery > 0 || h.state.LeftStatus == StatusInCase {
		h.state.LeftBattery = leftBattery
	}
	if rightBattery > 0 || h.state.RightStatus == StatusInCase {
		h.state.RightBattery = rightBattery
	}

	log.Printf("🔋 Battery: Left %d%%, Right %d%%", h.state.LeftBattery, h.state.RightBattery)
}

func (h *Handler) parseWearStatus(data []byte) {
	if len(data) < 5 {
		return
	}

	h.state.LeftStatus = EarbudStatus(data[3])
	h.state.RightStatus = EarbudStatus(data[4])

	log.Printf("👂 Status: Left=%s, Right=%s", h.state.LeftStatus, h.state.RightStatus)
}

func (h *Handler) parseANCMode(data []byte) {
	if len(data) < 2 {
		return
	}

	h.state.ANCMode = ANCMode(data[1])
	log.Printf("🎧 ANC Mode: %s", h.state.ANCMode)
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
