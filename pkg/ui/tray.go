package ui

import (
	"fmt"
	"log"
	"sync"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/protocol"
)

type TrayManager struct {
	mStatus *systray.MenuItem
	mQuit   *systray.MenuItem

	// Device-specific menu items
	gameBudsMenu  *systray.MenuItem
	gameBudsLeft  *systray.MenuItem
	gameBudsRight *systray.MenuItem
	gameBudsANC   *systray.MenuItem
	razerMenu     *systray.MenuItem
	razerBattery  *systray.MenuItem
	razerCharging *systray.MenuItem

	// State tracking
	devices map[string]protocol.DeviceState
	mu      sync.RWMutex
}

func NewTrayManager() *TrayManager {
	return &TrayManager{
		devices: make(map[string]protocol.DeviceState),
	}
}

func (t *TrayManager) Initialize() {
	systray.SetTitle("🎧")
	systray.SetTooltip("Battery Monitor")

	// Status item
	t.mStatus = systray.AddMenuItem("Initializing...", "Connection status")
	t.mStatus.Disable()

	systray.AddSeparator()

	// GameBuds section (initially hidden)
	t.gameBudsMenu = systray.AddMenuItem("🎧 GameBuds", "SteelSeries Arctis GameBuds")
	t.gameBudsMenu.Disable()
	t.gameBudsLeft = systray.AddMenuItem("  Left: --", "Left earbud battery")
	t.gameBudsLeft.Disable()
	t.gameBudsRight = systray.AddMenuItem("  Right: --", "Right earbud battery")
	t.gameBudsRight.Disable()
	t.gameBudsANC = systray.AddMenuItem("  ANC: Unknown", "Noise cancellation mode")
	t.gameBudsANC.Disable()

	systray.AddSeparator()

	// Razer section (initially hidden)
	t.razerMenu = systray.AddMenuItem("🖱️ Razer Device", "Razer Device")
	t.razerMenu.Disable()
	t.razerBattery = systray.AddMenuItem("  Battery: --", "Battery level")
	t.razerBattery.Disable()
	t.razerCharging = systray.AddMenuItem("  Charging: --", "Charging status")
	t.razerCharging.Disable()

	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "Quit goarctis")
}

func (t *TrayManager) SetStatus(status string) {
	t.mStatus.SetTitle(status)
}

func (t *TrayManager) UpdateDeviceState(deviceID string, state protocol.DeviceState) {
	t.mu.Lock()
	t.devices[deviceID] = state
	t.mu.Unlock()

	log.Printf("State updated for %s: %s", deviceID, state)

	// Update device-specific UI
	switch state.DeviceType {
	case "steelseries_gamebuds":
		t.updateGameBuds(state)
	case "razer_deathadder":
		t.updateRazer(state)
	}

	// Update tray icon and tooltip
	t.updateTrayIcon()
}

func (t *TrayManager) updateGameBuds(state protocol.DeviceState) {
	// Show GameBuds menu
	t.gameBudsMenu.SetTitle("🎧 GameBuds")
	t.gameBudsMenu.Enable()

	// Update Left battery
	leftText := formatGameBudsBattery(state.LeftBattery, state.LeftStatus, "Left")
	t.gameBudsLeft.SetTitle("  " + leftText)
	t.gameBudsLeft.Enable()

	// Update Right battery
	rightText := formatGameBudsBattery(state.RightBattery, state.RightStatus, "Right")
	t.gameBudsRight.SetTitle("  " + rightText)
	t.gameBudsRight.Enable()

	// Update ANC mode
	ancText := "  ANC: Unknown"
	if state.ANCMode != nil {
		ancIcon := getANCIcon(*state.ANCMode)
		ancText = fmt.Sprintf("  %s ANC: %s", ancIcon, state.ANCMode.String())
	}
	t.gameBudsANC.SetTitle(ancText)
	t.gameBudsANC.Enable()
}

func (t *TrayManager) updateRazer(state protocol.DeviceState) {
	// Show Razer menu
	t.razerMenu.SetTitle("🖱️ Razer Device")
	t.razerMenu.Enable()

	// Update battery
	batteryText := "  Battery: --"
	if state.Battery != nil {
		batteryIcon := getBatteryIcon(*state.Battery)
		batteryText = fmt.Sprintf("  %s Battery: %d%%", batteryIcon, *state.Battery)
	}
	t.razerBattery.SetTitle(batteryText)
	t.razerBattery.Enable()

	// Update charging/wireless status
	chargingText := "  Mode: --"
	if state.IsCharging != nil {
		if *state.IsCharging {
			chargingText = "  ⚡ Mode: Charging"
		} else {
			chargingText = "  📡 Mode: Wireless"
		}
	}
	t.razerCharging.SetTitle(chargingText)
	t.razerCharging.Enable()
}

func (t *TrayManager) updateTrayIcon() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var gameBudsBattery int = -1
	var mouseBattery int = -1
	var tooltipParts []string
	hasGameBuds := false
	hasMouse := false

	for _, state := range t.devices {
		switch state.DeviceType {
		case "steelseries_gamebuds":
			hasGameBuds = true
			// Get the lower of the two earbud batteries
			// Prefer earbuds that are out/wearing, but fall back to any available battery
			var validBatteries []int
			var allBatteries []int

			if state.LeftBattery != nil && *state.LeftBattery > 0 {
				allBatteries = append(allBatteries, *state.LeftBattery)
				leftStatus := protocol.StatusInCase
				if state.LeftStatus != nil {
					leftStatus = *state.LeftStatus
				}
				// Only count if not in case
				if leftStatus != protocol.StatusInCase {
					validBatteries = append(validBatteries, *state.LeftBattery)
				}
			}

			if state.RightBattery != nil && *state.RightBattery > 0 {
				allBatteries = append(allBatteries, *state.RightBattery)
				rightStatus := protocol.StatusInCase
				if state.RightStatus != nil {
					rightStatus = *state.RightStatus
				}
				// Only count if not in case
				if rightStatus != protocol.StatusInCase {
					validBatteries = append(validBatteries, *state.RightBattery)
				}
			}

			// Find the lowest battery - prefer valid (out/wearing), fall back to all batteries
			batteriesToUse := validBatteries
			if len(batteriesToUse) == 0 {
				batteriesToUse = allBatteries
			}

			if len(batteriesToUse) > 0 {
				lowest := batteriesToUse[0]
				for _, bat := range batteriesToUse[1:] {
					if bat < lowest {
						lowest = bat
					}
				}
				gameBudsBattery = lowest
			}

			tooltipParts = append(tooltipParts, fmt.Sprintf("GameBuds: %s", state.String()))
		case "razer_deathadder":
			hasMouse = true
			if state.Battery != nil {
				mouseBattery = *state.Battery
			}
			tooltipParts = append(tooltipParts, fmt.Sprintf("Razer: %s", state.String()))
		}
	}

	// Build tray icon title with both battery levels
	var titleParts []string

	// Show GameBuds battery if device exists
	if hasGameBuds {
		if gameBudsBattery >= 0 {
			titleParts = append(titleParts, fmt.Sprintf("🎧 %d%%", gameBudsBattery))
		} else {
			titleParts = append(titleParts, "🎧 --")
		}
	}

	// Show Mouse battery if device exists
	if hasMouse {
		if mouseBattery >= 0 {
			titleParts = append(titleParts, fmt.Sprintf("🖱️ %d%%", mouseBattery))
		} else {
			titleParts = append(titleParts, "🖱️ --")
		}
	}

	// Update tray icon
	if len(titleParts) == 0 {
		systray.SetTitle("🎧")
		systray.SetTooltip("No devices connected")
	} else {
		title := ""
		for i, part := range titleParts {
			if i > 0 {
				title += " "
			}
			title += part
		}
		log.Printf("Setting tray title: %s", title)
		systray.SetTitle(title)

		// Update tooltip
		tooltip := ""
		for i, part := range tooltipParts {
			if i > 0 {
				tooltip += " | "
			}
			tooltip += part
		}
		systray.SetTooltip(tooltip)
	}
}

func formatGameBudsBattery(battery *int, status *protocol.EarbudStatus, side string) string {
	if battery == nil && status == nil {
		return fmt.Sprintf("🎧 %s: --", side)
	}

	batteryVal := 0
	if battery != nil {
		batteryVal = *battery
	}

	// If status is nil, treat as unknown/default
	if status == nil {
		if batteryVal > 0 {
			return fmt.Sprintf("%s %s: %d%%", getBatteryIcon(batteryVal), side, batteryVal)
		}
		return fmt.Sprintf("🎧 %s: --", side)
	}

	statusVal := *status
	switch statusVal {
	case protocol.StatusInCase:
		if batteryVal > 0 {
			return fmt.Sprintf("🔋 %s: %d%% - Charging", side, batteryVal)
		}
		return fmt.Sprintf("📦 %s: In Case", side)
	case protocol.StatusOut:
		if batteryVal > 0 {
			return fmt.Sprintf("%s %s: %d%% - Out", getBatteryIcon(batteryVal), side, batteryVal)
		}
		return fmt.Sprintf("🎧 %s: Out", side)
	case protocol.StatusWorn:
		if batteryVal > 0 {
			return fmt.Sprintf("%s %s: %d%% - Wearing", getBatteryIcon(batteryVal), side, batteryVal)
		}
		return fmt.Sprintf("👂 %s: Wearing", side)
	default:
		if batteryVal > 0 {
			return fmt.Sprintf("%s %s: %d%%", getBatteryIcon(batteryVal), side, batteryVal)
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
