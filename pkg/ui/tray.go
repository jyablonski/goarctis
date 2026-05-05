package ui

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/getlantern/systray"
	"github.com/jyablonski/goarctis/pkg/docker"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/system"
	"github.com/jyablonski/goarctis/pkg/version"
)

type TrayConfig struct {
	DisableGameBuds bool
	DisableRazer    bool
	DisableHyperX   bool
	DisableSystem   bool
}

type TrayManager struct {
	mStatus *systray.MenuItem
	mQuit   *systray.MenuItem

	gameBudsMenu  *systray.MenuItem
	gameBudsLeft  *systray.MenuItem
	gameBudsRight *systray.MenuItem
	gameBudsANC   *systray.MenuItem
	razerMenu     *systray.MenuItem
	razerBattery  *systray.MenuItem
	razerCharging *systray.MenuItem

	hyperxMenu    *systray.MenuItem
	hyperxBattery *systray.MenuItem

	dockerMenu    *systray.MenuItem
	dockerStatus  *systray.MenuItem
	dockerStopAll *systray.MenuItem

	systemMenu   *systray.MenuItem
	systemCPU    *systray.MenuItem
	systemPeak   *systray.MenuItem
	systemMemory *systray.MenuItem

	config TrayConfig

	devices     map[string]protocol.DeviceState
	dockerState docker.DockerState
	systemState system.State
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

	t.mStatus = systray.AddMenuItem("Initializing...", "Connection status")
	t.mStatus.Disable()

	// GameBuds section. Hidden until the first state update arrives so the
	// section disappears entirely when no GameBuds device is discovered.
	if !cfg.DisableGameBuds {
		systray.AddSeparator()
		t.gameBudsMenu = addDisabledHiddenMenuItem("🎧 GameBuds", "SteelSeries Arctis GameBuds")
		t.gameBudsLeft = addDisabledHiddenMenuItem("  Left: --", "Left earbud battery")
		t.gameBudsRight = addDisabledHiddenMenuItem("  Right: --", "Right earbud battery")
		t.gameBudsANC = addDisabledHiddenMenuItem("  ANC: Unknown", "Noise cancellation mode")
	}

	if !cfg.DisableRazer {
		systray.AddSeparator()
		t.razerMenu = addDisabledHiddenMenuItem("🖱️ Razer Device", "Razer Device")
		t.razerBattery = addDisabledHiddenMenuItem("  Battery: --", "Battery level")
		t.razerCharging = addDisabledHiddenMenuItem("  Charging: --", "Charging status")
	}

	// HyperX section (only added if not disabled). Hidden at startup until the
	// first poll returns a connected state.
	if !cfg.DisableHyperX {
		systray.AddSeparator()
		t.hyperxMenu = addDisabledHiddenMenuItem("🎧 HyperX Cloud Alpha Wireless", "HyperX Cloud Alpha Wireless")
		t.hyperxBattery = addDisabledHiddenMenuItem("  Battery: --", "Battery level")
	}

	if !cfg.DisableSystem {
		systray.AddSeparator()
		t.systemMenu = systray.AddMenuItem("🖥️ System", "System resource utilization")
		t.systemMenu.Disable()
		t.systemCPU = systray.AddMenuItem("  ⚙️ CPU: checking...", "CPU utilization")
		t.systemCPU.Disable()
		t.systemPeak = systray.AddMenuItem("  🔥 CPU Peak: --", "Recent CPU peak")
		t.systemPeak.Disable()
		t.systemPeak.Hide()
		t.systemMemory = systray.AddMenuItem("  🧠 Memory: checking...", "Memory utilization")
		t.systemMemory.Disable()
	}

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

	switch state.DeviceType {
	case protocol.DeviceTypeSteelSeriesGameBuds:
		t.updateGameBuds(state)
	case protocol.DeviceTypeRazerDeathAdder:
		t.updateRazer(state)
	case protocol.DeviceTypeHyperXCloudAlpha:
		t.updateHyperX(state)
	}

	t.updateTrayIcon()
}

func addDisabledHiddenMenuItem(title, tooltip string) *systray.MenuItem {
	item := systray.AddMenuItem(title, tooltip)
	item.Disable()
	item.Hide()
	return item
}

func (t *TrayManager) gameBudsItems() []*systray.MenuItem {
	return []*systray.MenuItem{t.gameBudsMenu, t.gameBudsLeft, t.gameBudsRight, t.gameBudsANC}
}

func (t *TrayManager) razerItems() []*systray.MenuItem {
	return []*systray.MenuItem{t.razerMenu, t.razerBattery, t.razerCharging}
}

func (t *TrayManager) hyperxItems() []*systray.MenuItem {
	return []*systray.MenuItem{t.hyperxMenu, t.hyperxBattery}
}

func showMenuItems(items ...*systray.MenuItem) {
	for _, item := range items {
		if item != nil {
			item.Show()
		}
	}
}

func hideMenuItems(items ...*systray.MenuItem) {
	for _, item := range items {
		if item != nil {
			item.Hide()
		}
	}
}

func (t *TrayManager) updateGameBuds(state protocol.DeviceState) {
	if t.gameBudsMenu == nil {
		return
	}

	// The GameBuds handler hardcodes IsConnected = true as soon as a dongle
	// is found, even before any earbud data has arrived. That leaves a ghost
	// "Left: -- / Right: -- / ANC: Unknown" section whenever the dongle is
	// plugged in but the earbuds aren't active. Require at least one piece
	// of real data before showing the section.
	if !isVisibleGameBudsState(state) {
		hideMenuItems(t.gameBudsItems()...)
		return
	}

	showMenuItems(t.gameBudsItems()...)

	t.gameBudsMenu.SetTitle("🎧 GameBuds")
	t.gameBudsMenu.Enable()

	leftText := formatGameBudsBattery(state.LeftBattery, state.LeftStatus, "Left")
	t.gameBudsLeft.SetTitle("  " + leftText)
	t.gameBudsLeft.Enable()

	rightText := formatGameBudsBattery(state.RightBattery, state.RightStatus, "Right")
	t.gameBudsRight.SetTitle("  " + rightText)
	t.gameBudsRight.Enable()

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

	if !state.IsConnected {
		hideMenuItems(t.razerItems()...)
		return
	}

	showMenuItems(t.razerItems()...)

	t.razerMenu.SetTitle("🖱️ Razer Device")
	t.razerMenu.Enable()

	t.razerBattery.SetTitle(formatBatteryMenuTitle(state.Battery))
	t.razerBattery.Enable()

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

func (t *TrayManager) updateHyperX(state protocol.DeviceState) {
	if t.hyperxMenu == nil {
		return
	}

	if !state.IsConnected {
		hideMenuItems(t.hyperxItems()...)
		return
	}

	showMenuItems(t.hyperxItems()...)
	t.hyperxMenu.Enable()

	t.hyperxBattery.SetTitle(formatHyperXBatteryMenuTitle(state))
	t.hyperxBattery.Enable()
}

func (t *TrayManager) updateTrayIcon() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	gameBudsBattery := -1
	mouseBattery := -1
	hyperxBattery := -1
	hyperxCharging := false
	var tooltipParts []string
	hasGameBuds := false
	hasMouse := false
	hasHyperX := false

	for _, state := range t.devices {
		switch state.DeviceType {
		case protocol.DeviceTypeSteelSeriesGameBuds:
			if !isVisibleGameBudsState(state) {
				continue
			}
			hasGameBuds = true
			gameBudsBattery = gameBudsTrayBattery(state)
			tooltipParts = append(tooltipParts, fmt.Sprintf("GameBuds: %s", state.String()))
		case protocol.DeviceTypeRazerDeathAdder:
			if !state.IsConnected {
				continue
			}
			hasMouse = true
			if state.Battery != nil {
				mouseBattery = *state.Battery
			}
			tooltipParts = append(tooltipParts, fmt.Sprintf("Razer: %s", state.String()))
		case protocol.DeviceTypeHyperXCloudAlpha:
			if !state.IsConnected {
				continue
			}
			hasHyperX = true
			if state.IsCharging != nil && *state.IsCharging {
				hyperxCharging = true
			}
			if state.Battery != nil {
				hyperxBattery = *state.Battery
			}
			tooltipParts = append(tooltipParts, fmt.Sprintf("HyperX: %s", state.String()))
		}
	}

	var titleParts []string

	if hasGameBuds {
		titleParts = append(titleParts, formatTrayBattery("🎧", gameBudsBattery, false))
	}

	if hasHyperX {
		titleParts = append(titleParts, formatTrayBattery("🎧", hyperxBattery, hyperxCharging))
	}

	if hasMouse {
		titleParts = append(titleParts, formatTrayBattery("🖱️", mouseBattery, false))
	}

	dockerCount := t.dockerState.RunningCount()
	if dockerCount > 0 {
		titleParts = append(titleParts, fmt.Sprintf("🐳 %d", dockerCount))
		tooltipParts = append(tooltipParts, fmt.Sprintf("Docker: %d container(s)", dockerCount))
	}

	if t.systemState.Available {
		if t.systemState.CPUSpiking && t.systemState.CPUPercent != nil {
			titleParts = append(titleParts, formatSystemCPUTitle(*t.systemState.CPUPercent))
		}
		if t.systemState.MemoryPercent != nil {
			titleParts = append(titleParts, formatSystemMemoryTitle(*t.systemState.MemoryPercent))
		}
		if tooltip := formatSystemTooltip(t.systemState); tooltip != "" {
			tooltipParts = append(tooltipParts, tooltip)
		}
	}

	if len(titleParts) == 0 {
		systray.SetTitle("🎧")
		systray.SetTooltip(fmt.Sprintf("No devices connected (v%s)", version.Version))
	} else {
		title := strings.Join(titleParts, " ")
		log.Printf("Setting tray title: %s", title)
		systray.SetTitle(title)

		tooltip := strings.Join(tooltipParts, " | ")
		tooltip += fmt.Sprintf(" (v%s)", version.Version)
		systray.SetTooltip(tooltip)
	}
}

func (t *TrayManager) UpdateDockerState(state docker.DockerState) {
	t.mu.Lock()
	t.dockerState = state
	t.mu.Unlock()

	if t.dockerMenu == nil {
		return
	}
	defer t.updateTrayIcon()

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

func (t *TrayManager) UpdateSystemState(state system.State) {
	t.mu.Lock()
	t.systemState = state
	t.mu.Unlock()

	if t.systemMenu == nil {
		return
	}
	defer t.updateTrayIcon()

	if !state.Available {
		t.systemMenu.SetTitle("🖥️ System (unavailable)")
		t.systemCPU.SetTitle("  ⚙️ CPU: unavailable")
		t.systemMemory.SetTitle("  🧠 Memory: unavailable")
		t.systemPeak.Hide()
		return
	}

	t.systemMenu.SetTitle("🖥️ System")
	t.systemCPU.SetTitle(formatSystemCPUMenuTitle(state))
	t.systemMemory.SetTitle(formatSystemMemoryMenuTitle(state))

	if state.CPUPeakPercent == nil {
		t.systemPeak.Hide()
	} else {
		t.systemPeak.SetTitle(formatSystemPeakMenuTitle(state))
		t.systemPeak.Show()
		t.systemPeak.Enable()
	}

	log.Printf("System state updated: cpu=%s, memory=%s",
		formatOptionalPercent(state.CPUPercent), formatOptionalPercent(state.MemoryPercent))
}

func (t *TrayManager) DockerStopAllChannel() <-chan struct{} {
	return t.dockerStopAll.ClickedCh
}

func joinNames(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s +%d more", strings.Join(names[:max], ", "), len(names)-max)
}

func isVisibleGameBudsState(state protocol.DeviceState) bool {
	return state.IsConnected && (state.LeftBattery != nil || state.RightBattery != nil ||
		state.LeftStatus != nil || state.RightStatus != nil || state.ANCMode != nil)
}

func gameBudsTrayBattery(state protocol.DeviceState) int {
	validBatteries := make([]int, 0, 2)
	allBatteries := make([]int, 0, 2)

	collectEarbudBattery(&validBatteries, &allBatteries, state.LeftBattery, state.LeftStatus)
	collectEarbudBattery(&validBatteries, &allBatteries, state.RightBattery, state.RightStatus)

	if len(validBatteries) > 0 {
		return lowestBattery(validBatteries)
	}
	return lowestBattery(allBatteries)
}

func collectEarbudBattery(validBatteries, allBatteries *[]int, battery *int, status *protocol.EarbudStatus) {
	if battery == nil || *battery <= 0 {
		return
	}

	*allBatteries = append(*allBatteries, *battery)
	if status != nil && *status != protocol.StatusInCase {
		*validBatteries = append(*validBatteries, *battery)
	}
}

func lowestBattery(batteries []int) int {
	if len(batteries) == 0 {
		return -1
	}

	lowest := batteries[0]
	for _, battery := range batteries[1:] {
		if battery < lowest {
			lowest = battery
		}
	}
	return lowest
}

func formatBatteryMenuTitle(battery *int) string {
	if battery == nil {
		return "  Battery: --"
	}
	return fmt.Sprintf("  %s Battery: %d%%", getBatteryIcon(*battery), *battery)
}

func formatHyperXBatteryMenuTitle(state protocol.DeviceState) string {
	if state.IsCharging != nil && *state.IsCharging && state.Battery == nil {
		return "  ⚡ Battery: Charging"
	}
	return formatBatteryMenuTitle(state.Battery)
}

func formatTrayBattery(icon string, battery int, charging bool) string {
	switch {
	case battery >= 0:
		return fmt.Sprintf("%s %d%%", icon, battery)
	case charging:
		return fmt.Sprintf("%s ⚡", icon)
	default:
		return fmt.Sprintf("%s --", icon)
	}
}

func formatSystemCPUMenuTitle(state system.State) string {
	if state.CPUPercent == nil {
		return "  ⚙️ CPU: --"
	}
	icon := "⚙️"
	if state.CPUSpiking {
		icon = "🔥"
	}
	return fmt.Sprintf("  %s CPU: %d%%", icon, *state.CPUPercent)
}

func formatSystemPeakMenuTitle(state system.State) string {
	if state.CPUPeakPercent == nil {
		return "  🔥 CPU Peak: --"
	}
	return fmt.Sprintf("  🔥 CPU Peak: %d%% last 60s", *state.CPUPeakPercent)
}

func formatSystemMemoryMenuTitle(state system.State) string {
	if state.MemoryPercent == nil {
		return "  🧠 Memory: --"
	}
	if state.MemoryTotalBytes == 0 {
		return fmt.Sprintf("  🧠 Memory: %d%%", *state.MemoryPercent)
	}
	return fmt.Sprintf("  🧠 Memory: %d%% (%s / %s)",
		*state.MemoryPercent,
		formatBytes(state.MemoryUsedBytes),
		formatBytes(state.MemoryTotalBytes),
	)
}

func formatSystemCPUTitle(percent int) string {
	return fmt.Sprintf("🔥 %d%%", percent)
}

func formatSystemMemoryTitle(percent int) string {
	return fmt.Sprintf("🧠 %d%%", percent)
}

func formatSystemTooltip(state system.State) string {
	var parts []string
	if state.CPUPercent != nil {
		cpu := fmt.Sprintf("CPU: %d%%", *state.CPUPercent)
		if state.CPUSpiking {
			cpu += " spiking"
		}
		parts = append(parts, cpu)
	}
	if state.MemoryPercent != nil {
		parts = append(parts, fmt.Sprintf("Memory: %d%%", *state.MemoryPercent))
	}
	if len(parts) == 0 {
		return ""
	}
	return "System: " + strings.Join(parts, ", ")
}

func formatBytes(bytes uint64) string {
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
}

func formatOptionalPercent(percent *int) string {
	if percent == nil {
		return "--"
	}
	return fmt.Sprintf("%d%%", *percent)
}

func formatGameBudsBattery(battery *int, status *protocol.EarbudStatus, side string) string {
	if battery == nil && status == nil {
		return fmt.Sprintf("🎧 %s: --", side)
	}

	batteryVal := 0
	if battery != nil {
		batteryVal = *battery
	}

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
