package ui

import (
	"fmt"
	"log"
	"sync"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/docker"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/version"
)

// TrayConfig holds configuration for which sections to display in the tray menu.
type TrayConfig struct {
	DisableGameBuds bool
	DisableRazer    bool
}

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

	// Docker menu items
	dockerMenu    *systray.MenuItem
	dockerStatus  *systray.MenuItem
	dockerStopAll *systray.MenuItem

	// Configuration
	config TrayConfig

	// State tracking
	devices     map[string]protocol.DeviceState
	dockerState docker.DockerState
	mu          sync.RWMutex
}

func NewTrayManager() *TrayManager {
	return &TrayManager{
		devices: make(map[string]protocol.DeviceState),
	}
}

func (t *TrayManager) Initialize(cfg TrayConfig) {
	t.config = cfg

	systray.SetTitle("🎧")
	systray.SetTooltip(fmt.Sprintf("Battery Monitor (v%s)", version.Version))

	// Status item
	t.mStatus = systray.AddMenuItem("Initializing...", "Connection status")
	t.mStatus.Disable()

	// GameBuds section (only added if not disabled)
	if !cfg.DisableGameBuds {
		systray.AddSeparator()
		t.gameBudsMenu = systray.AddMenuItem("🎧 GameBuds", "SteelSeries Arctis GameBuds")
		t.gameBudsMenu.Disable()
		t.gameBudsLeft = systray.AddMenuItem("  Left: --", "Left earbud battery")
		t.gameBudsLeft.Disable()
		t.gameBudsRight = systray.AddMenuItem("  Right: --", "Right earbud battery")
		t.gameBudsRight.Disable()
		t.gameBudsANC = systray.AddMenuItem("  ANC: Unknown", "Noise cancellation mode")
		t.gameBudsANC.Disable()
	}

	// Razer section (only added if not disabled)
	if !cfg.DisableRazer {
		systray.AddSeparator()
		t.razerMenu = systray.AddMenuItem("🖱️ Razer Device", "Razer Device")
		t.razerMenu.Disable()
		t.razerBattery = systray.AddMenuItem("  Battery: --", "Battery level")
		t.razerBattery.Disable()
		t.razerCharging = systray.AddMenuItem("  Charging: --", "Charging status")
		t.razerCharging.Disable()
	}

	// Docker section
	systray.AddSeparator()
	t.dockerMenu = systray.AddMenuItem("🐳 Docker", "Docker container status")
	t.dockerMenu.Disable()
	t.dockerStatus = systray.AddMenuItem("  Containers: checking...", "Running container count")
	t.dockerStatus.Disable()
	t.dockerStopAll = systray.AddMenuItem("  Stop All Containers", "Stop all running Docker containers")
	t.dockerStopAll.Hide()

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
	if t.gameBudsMenu == nil {
		return
	}
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
	if t.razerMenu == nil {
		return
	}
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

	// Show Docker container count if any are running
	dockerCount := t.dockerState.RunningCount()
	if dockerCount > 0 {
		titleParts = append(titleParts, fmt.Sprintf("🐳 %d", dockerCount))
		tooltipParts = append(tooltipParts, fmt.Sprintf("Docker: %d container(s)", dockerCount))
	}

	// Update tray icon
	if len(titleParts) == 0 {
		systray.SetTitle("🎧")
		systray.SetTooltip(fmt.Sprintf("No devices connected (v%s)", version.Version))
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

		// Update tooltip with version
		tooltip := ""
		for i, part := range tooltipParts {
			if i > 0 {
				tooltip += " | "
			}
			tooltip += part
		}
		tooltip += fmt.Sprintf(" (v%s)", version.Version)
		systray.SetTooltip(tooltip)
	}
}

// UpdateDockerState updates the Docker section of the tray menu
func (t *TrayManager) UpdateDockerState(state docker.DockerState) {
	t.mu.Lock()
	t.dockerState = state
	t.mu.Unlock()

	if t.dockerMenu == nil {
		return
	}

	if !state.Available {
		t.dockerMenu.SetTitle("🐳 Docker (unavailable)")
		t.dockerStatus.SetTitle("  Docker not running")
		t.dockerStopAll.Hide()
		return
	}

	count := state.RunningCount()
	if count == 0 {
		t.dockerMenu.SetTitle("🐳 Docker")
		t.dockerStatus.SetTitle("  No containers running")
		t.dockerStopAll.Hide()
	} else {
		t.dockerMenu.SetTitle(fmt.Sprintf("🐳 Docker (%d running)", count))
		// List container names
		names := make([]string, 0, len(state.Containers))
		for _, c := range state.Containers {
			names = append(names, c.Name)
		}
		t.dockerStatus.SetTitle(fmt.Sprintf("  Running: %s", joinNames(names, 3)))
		t.dockerStopAll.Show()
		t.dockerStopAll.Enable()
	}

	log.Printf("Docker state updated: available=%v, containers=%d", state.Available, count)
}

// DockerStopAllChannel returns the channel that receives clicks on "Stop All Containers"
func (t *TrayManager) DockerStopAllChannel() <-chan struct{} {
	return t.dockerStopAll.ClickedCh
}

// joinNames joins container names, truncating with "+N more" if there are too many
func joinNames(names []string, max int) string {
	if len(names) <= max {
		result := ""
		for i, n := range names {
			if i > 0 {
				result += ", "
			}
			result += n
		}
		return result
	}
	result := ""
	for i := 0; i < max; i++ {
		if i > 0 {
			result += ", "
		}
		result += names[i]
	}
	result += fmt.Sprintf(" +%d more", len(names)-max)
	return result
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
