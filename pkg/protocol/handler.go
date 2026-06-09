package protocol

import (
	"errors"
	"fmt"
	"log"
)

const (
	DeviceTypeSteelSeriesGameBuds = "steelseries_gamebuds"
	DeviceTypeRazer               = "razer_deathadder"
	DeviceTypeHyperXCloudAlpha    = "hyperx_cloud_alpha_wireless"

	ReportBattery    = 0xB7
	ReportWearStatus = 0xB5
	ReportANCMode    = 0xBD
	ReportInEarEvent = 0xC6

	// ReportStatus (0xB0) is the consolidated status report the dongle returns
	// when polled with the 0xB0 command. Layout (after the 0xB0 report byte):
	//   byte[3] = left wear status, byte[4] = right wear status (see EarbudStatus)
	//   byte[5] = left battery %,   byte[6] = right battery %
	//   byte[9] = ANC mode (see ANCMode)
	ReportStatus = 0xB0
)

var (
	ErrEmptyReport                   = errors.New("empty report")
	ErrCommandEncodingNotImplemented = errors.New("command encoding not yet implemented")
)

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
	Warning         string
}

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

// Equal compares two DeviceState values by the values behind their optional
// fields. Direct struct comparison is not suitable here because many fields are
// pointers that are rebuilt on each poll.
func (s DeviceState) Equal(other DeviceState) bool {
	return s.DeviceID == other.DeviceID &&
		s.DeviceType == other.DeviceType &&
		s.IsConnected == other.IsConnected &&
		s.FirmwareVersion == other.FirmwareVersion &&
		s.Warning == other.Warning &&
		pointerEqual(s.Battery, other.Battery) &&
		pointerEqual(s.LeftBattery, other.LeftBattery) &&
		pointerEqual(s.RightBattery, other.RightBattery) &&
		pointerEqual(s.DockBattery, other.DockBattery) &&
		pointerEqual(s.IsCharging, other.IsCharging) &&
		pointerEqual(s.LeftStatus, other.LeftStatus) &&
		pointerEqual(s.RightStatus, other.RightStatus) &&
		pointerEqual(s.ANCMode, other.ANCMode)
}

func (s DeviceState) String() string {
	switch s.DeviceType {
	case DeviceTypeSteelSeriesGameBuds:
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

		out := fmt.Sprintf("L:%s | R:%s | ANC:%s", leftStr, rightStr, ancStr)
		if s.DockBattery != nil {
			out += fmt.Sprintf(" | Case:%d%%", *s.DockBattery)
		}
		return out
	case DeviceTypeRazer:
		batteryStr := "--"
		chargingStr := ""
		if s.Warning != "" {
			return s.Warning
		}
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

type Handler struct {
	state    DeviceState
	onChange func(DeviceState)
}

func NewHandler() *Handler {
	return &Handler{
		state: DeviceState{
			DeviceType:  DeviceTypeSteelSeriesGameBuds,
			IsConnected: true,
		},
	}
}

func (h *Handler) SetOnChange(callback func(DeviceState)) {
	h.onChange = callback
}

func (h *Handler) ParseReport(data []byte) error {
	if len(data) == 0 {
		return ErrEmptyReport
	}

	reportID := data[0]
	oldState := h.copyState()

	// Handle reports with ID 0x00 - might be reports without explicit ID or different format
	if reportID == 0x00 && len(data) >= 2 {
		// Some HID devices put the report ID in byte 1 or embed it differently
		for offset := 1; offset < len(data) && offset < 4; offset++ {
			possibleID := data[offset]
			if possibleID == ReportBattery || possibleID == ReportWearStatus ||
				possibleID == ReportANCMode || possibleID == ReportInEarEvent {
				log.Printf("🎧 GameBuds: Found embedded report ID 0x%02X at offset %d, data: %x", possibleID, offset, data)
				reportID = possibleID
				break
			}
		}

		// If no embedded report ID found, this might be a button/control event
		if reportID == 0x00 {
			// The pattern 00020000 / 00000000 suggests button press/release
			// This is likely a media control event (pause/play) - not a status report
			if len(data) >= 2 && data[1] == 0x02 {
				log.Printf("🎧 GameBuds: Report 0x00 received (button/control event - pause detected): %x", data)
			} else {
				log.Printf("🎧 GameBuds: Report 0x00 received (button/control event): %x", data)
			}
			return nil
		}
	}

	switch reportID {
	case ReportStatus:
		log.Printf("🎧 GameBuds: Parsing status report (0x%02X)", reportID)
		h.parseStatus(data)
	case ReportBattery:
		log.Printf("🎧 GameBuds: Parsing battery report (0x%02X)", reportID)
		h.parseBattery(data)
	case ReportWearStatus:
		log.Printf("🎧 GameBuds: Parsing wear status report (0x%02X)", reportID)
		h.parseWearStatus(data)
	case ReportANCMode:
		log.Printf("🎧 GameBuds: Parsing ANC mode report (0x%02X)", reportID)
		h.parseANCMode(data)
	case ReportInEarEvent:
		log.Printf("🎧 GameBuds: Parsing in-ear event report (0x%02X)", reportID)
		h.parseInEarEvent(data)
	default:
		log.Printf("🎧 GameBuds: Unknown report 0x%02X: %x", reportID, data)
		return nil
	}

	if h.onChange != nil && !oldState.Equal(h.state) {
		log.Printf("🎧 GameBuds: State changed, triggering callback")
		h.onChange(h.state)
	} else if h.onChange != nil {
		log.Printf("🎧 GameBuds: State unchanged after parsing report 0x%02X", reportID)
	}

	return nil
}

func (h *Handler) copyState() DeviceState {
	state := h.state
	copy := DeviceState{
		DeviceID:        state.DeviceID,
		DeviceType:      state.DeviceType,
		IsConnected:     state.IsConnected,
		FirmwareVersion: state.FirmwareVersion,
		Warning:         state.Warning,
		Battery:         clonePtr(state.Battery),
		LeftBattery:     clonePtr(state.LeftBattery),
		RightBattery:    clonePtr(state.RightBattery),
		DockBattery:     clonePtr(state.DockBattery),
		IsCharging:      clonePtr(state.IsCharging),
		LeftStatus:      clonePtr(state.LeftStatus),
		RightStatus:     clonePtr(state.RightStatus),
		ANCMode:         clonePtr(state.ANCMode),
	}
	return copy
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

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

	offset := 1
	if len(data) > 0 && data[0] == 0xB7 {
		offset = 1
	} else if len(data) > 1 && data[1] == 0xB7 {
		offset = 2
	}

	if len(data) < offset+2 {
		return
	}

	leftBattery := int(data[offset])
	rightBattery := int(data[offset+1])

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

// parseStatus parses the consolidated 0xB0 status report returned by the
// dongle. Layout (after the 0xB0 report byte):
//
//	byte[3]/byte[4] = left/right wear status (EarbudStatus)
//	byte[5]/byte[6] = left/right battery %  (0xFF = unknown)
//	byte[9]         = ANC mode (ANCMode)
//
// Carrying ANC here means it populates on the first poll, rather than only when
// an unsolicited 0xBD change-event arrives.
func (h *Handler) parseStatus(data []byte) {
	if len(data) < 7 {
		return
	}

	leftStatus := EarbudStatus(data[3])
	rightStatus := EarbudStatus(data[4])
	if leftStatus >= StatusInCase && leftStatus <= StatusWorn {
		h.state.LeftStatus = &leftStatus
	}
	if rightStatus >= StatusInCase && rightStatus <= StatusWorn {
		h.state.RightStatus = &rightStatus
	}

	if left := int(data[5]); left <= 100 {
		h.state.LeftBattery = &left
	}
	if right := int(data[6]); right <= 100 {
		h.state.RightBattery = &right
	}

	if len(data) >= 10 {
		if anc := ANCMode(data[9]); anc >= ANCOff && anc <= ANCActive {
			h.state.ANCMode = &anc
		}
	}

	h.state.IsConnected = true

	log.Printf("🔋 GameBuds status: Left %d%% (%s), Right %d%% (%s)",
		int(data[5]), leftStatus, int(data[6]), rightStatus)
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
	switch event {
	case 1:
		log.Println("👂 Earbud removed from ear")
	case 0:
		log.Println("👂 Earbud placed in ear")
	}
}

// SetDockBattery updates the charging case battery level. The case is a
// separate USB device (PID 0x230c) from the dongle, so this is driven by its
// own poll rather than the dongle's 0xB0 status report. A value of 0xFF (255)
// or out-of-range reading is treated as unknown and ignored.
func (h *Handler) SetDockBattery(battery int) {
	if battery < 0 || battery > 100 {
		return
	}

	oldState := h.copyState()
	h.state.DockBattery = &battery
	h.state.IsConnected = true

	if h.onChange != nil && !oldState.Equal(h.state) {
		h.onChange(h.state)
	}
}

func (h *Handler) GetState() DeviceState {
	return h.state
}

func (h *Handler) EncodeCommand(command string, params ...interface{}) ([]byte, error) {
	// TODO: Implement when we figure out how to send commands
	return nil, ErrCommandEncodingNotImplemented
}
