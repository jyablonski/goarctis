package ui

import (
	"fmt"
	"log"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/protocol"
)

type TrayManager struct {
	mStatus   *systray.MenuItem
	mBatteryL *systray.MenuItem
	mBatteryR *systray.MenuItem
	mANCMode  *systray.MenuItem
	mQuit     *systray.MenuItem
}

func NewTrayManager() *TrayManager {
	return &TrayManager{}
}

func (t *TrayManager) Initialize() {
	systray.SetTitle("🎧")
	// systray.SetIcon(IconData)
	systray.SetTooltip("Arctis GameBuds Manager")

	// Create menu items
	t.mStatus = systray.AddMenuItem("Initializing...", "Connection status")
	t.mStatus.Disable()

	systray.AddSeparator()

	t.mBatteryL = systray.AddMenuItem("Left: --", "Left earbud battery")
	t.mBatteryL.Disable()

	t.mBatteryR = systray.AddMenuItem("Right: --", "Right earbud battery")
	t.mBatteryR.Disable()

	systray.AddSeparator()

	t.mANCMode = systray.AddMenuItem("ANC: Unknown", "Noise cancellation mode")
	t.mANCMode.Disable()

	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "Quit goarctis")
}

func (t *TrayManager) SetStatus(status string) {
	t.mStatus.SetTitle(status)
}

func (t *TrayManager) UpdateState(state protocol.DeviceState) {
	log.Printf("State updated: %s", state)

	// Build battery display strings
	leftText := formatBatteryDisplay(state.LeftBattery, state.LeftStatus, "Left")
	rightText := formatBatteryDisplay(state.RightBattery, state.RightStatus, "Right")

	t.mBatteryL.SetTitle(leftText)
	t.mBatteryR.SetTitle(rightText)

	// Update ANC mode
	ancIcon := getANCIcon(state.ANCMode)
	t.mANCMode.SetTitle(fmt.Sprintf("%s %s", ancIcon, state.ANCMode))

	// Update system tray icon based on lowest battery (only count earbuds that are out/wearing)
	lowestBattery := 100
	if state.LeftStatus != protocol.StatusInCase && state.LeftBattery > 0 {
		lowestBattery = state.LeftBattery
	}
	if state.RightStatus != protocol.StatusInCase && state.RightBattery > 0 && state.RightBattery < lowestBattery {
		lowestBattery = state.RightBattery
	}

	if lowestBattery < 100 {
		systray.SetTitle(fmt.Sprintf("🎧 %d%%", lowestBattery))
	} else {
		systray.SetTitle("🎧")
	}

	// Update tooltip with full state
	systray.SetTooltip(state.String())
}

func formatBatteryDisplay(battery int, status protocol.EarbudStatus, side string) string {
	// Handle special cases
	switch status {
	case protocol.StatusInCase:
		if battery > 0 {
			return fmt.Sprintf("🔋 %s: %d%% - Charging", side, battery)
		}
		return fmt.Sprintf("📦 %s: In Case", side)
	case protocol.StatusOut:
		if battery > 0 {
			return fmt.Sprintf("%s %s: %d%% - Out", getBatteryIcon(battery), side, battery)
		}
		return fmt.Sprintf("🎧 %s: Out", side)
	case protocol.StatusWorn:
		if battery > 0 {
			return fmt.Sprintf("%s %s: %d%% - Wearing", getBatteryIcon(battery), side, battery)
		}
		return fmt.Sprintf("👂 %s: Wearing", side)
	default:
		// Unknown status
		if battery > 0 {
			return fmt.Sprintf("%s %s: %d%%", getBatteryIcon(battery), side, battery)
		}
		return fmt.Sprintf("🎧 %s: --", side)
	}
}

func (t *TrayManager) QuitChannel() <-chan struct{} {
	return t.mQuit.ClickedCh
}

func getBatteryIcon(level int) string {
	switch {
	case level >= 80:
		return "🔋"
	case level >= 50:
		return "🔋"
	case level >= 20:
		return "🪫"
	case level > 0:
		return "🪫"
	default:
		return "❓"
	}
}

func getANCIcon(mode protocol.ANCMode) string {
	switch mode {
	case protocol.ANCActive:
		return "🔇"
	case protocol.ANCTransparency:
		return "👂"
	case protocol.ANCOff:
		return "🔊"
	default:
		return "🎧"
	}
}
