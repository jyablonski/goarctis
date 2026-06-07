package ui

import (
	"bytes"
	"image"
	"image/png"
	"reflect"
	"testing"

	"github.com/jyablonski/goarctis/assets"
	"github.com/jyablonski/goarctis/pkg/docker"
	"github.com/jyablonski/goarctis/pkg/protocol"
	"github.com/jyablonski/goarctis/pkg/system"
)

func TestTrayIconPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(assets.TrayIconPNG))
	if err != nil {
		t.Fatalf("TrayIconPNG does not decode: %v", err)
	}
	if bounds := img.Bounds(); bounds.Dx() != 48 || bounds.Dy() != 48 {
		t.Fatalf("TrayIconPNG size = %dx%d, want 48x48", bounds.Dx(), bounds.Dy())
	}
	if opaqueBounds := opaqueImageBounds(img); opaqueBounds.Dx() < 44 || opaqueBounds.Dy() < 36 {
		t.Fatalf("TrayIconPNG opaque bounds = %dx%d, want at least 44x36", opaqueBounds.Dx(), opaqueBounds.Dy())
	}
}

func opaqueImageBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	found := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func TestGetBatteryIcon(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		expected string
	}{
		{"Full battery", 100, "🔋"},
		{"High battery", 85, "🔋"},
		{"Medium-high battery", 80, "🔋"},
		{"Medium battery", 60, "🔋"},
		{"Medium-low battery", 50, "🔋"},
		{"Low battery", 30, "🪫"},
		{"Very low battery", 20, "🪫"},
		{"Critical battery", 10, "🪫"},
		{"Almost dead", 1, "🪫"},
		{"Unknown/zero", 0, "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBatteryIcon(tt.level)
			if got != tt.expected {
				t.Errorf("getBatteryIcon(%d) = %v, want %v", tt.level, got, tt.expected)
			}
		})
	}
}

func TestGetANCIcon(t *testing.T) {
	tests := []struct {
		name     string
		mode     protocol.ANCMode
		expected string
	}{
		{"ANC Active", protocol.ANCActive, "🔇"},
		{"ANC Transparency", protocol.ANCTransparency, "👂"},
		{"ANC Off", protocol.ANCOff, "🔊"},
		{"Unknown mode", protocol.ANCMode(99), "🎧"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getANCIcon(tt.mode)
			if got != tt.expected {
				t.Errorf("getANCIcon(%v) = %v, want %v", tt.mode, got, tt.expected)
			}
		})
	}
}

func TestFormatGameBudsBattery(t *testing.T) {
	tests := []struct {
		name     string
		battery  *int
		status   *protocol.EarbudStatus
		side     string
		expected string
	}{
		{
			name:     "In case with battery",
			battery:  func() *int { b := 80; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusInCase; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 80% - Charging",
		},
		{
			name:     "In case without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusInCase; return &s }(),
			side:     "Right",
			expected: "📦 Right: In Case",
		},
		{
			name:     "Out of case with battery",
			battery:  func() *int { b := 75; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusOut; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 75% - Out",
		},
		{
			name:     "Out of case without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusOut; return &s }(),
			side:     "Right",
			expected: "🎧 Right: Out",
		},
		{
			name:     "Wearing with high battery",
			battery:  func() *int { b := 90; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Left",
			expected: "🔋 Left: 90% - Wearing",
		},
		{
			name:     "Wearing with low battery",
			battery:  func() *int { b := 15; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Right",
			expected: "🪫 Right: 15% - Wearing",
		},
		{
			name:     "Wearing without battery",
			battery:  func() *int { b := 0; return &b }(),
			status:   func() *protocol.EarbudStatus { s := protocol.StatusWorn; return &s }(),
			side:     "Left",
			expected: "👂 Left: Wearing",
		},
		{
			name:     "Unknown status with battery",
			battery:  func() *int { b := 50; return &b }(),
			status:   nil,
			side:     "Left",
			expected: "🔋 Left: 50%",
		},
		{
			name:     "Unknown status without battery",
			battery:  nil,
			status:   nil,
			side:     "Right",
			expected: "🎧 Right: --",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatGameBudsBattery(tt.battery, tt.status, tt.side)
			if got != tt.expected {
				t.Errorf("formatGameBudsBattery(%v, %v, %s) = %v, want %v",
					tt.battery, tt.status, tt.side, got, tt.expected)
			}
		})
	}
}

func TestNewTrayManager(t *testing.T) {
	manager := NewTrayManager()

	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}

	if manager.mStatus != nil {
		t.Error("mStatus should be nil before Initialize")
	}
	if manager.gameBudsMenu != nil {
		t.Error("gameBudsMenu should be nil before Initialize")
	}
	if manager.razerMenu != nil {
		t.Error("razerMenu should be nil before Initialize")
	}
	if manager.mQuit != nil {
		t.Error("mQuit should be nil before Initialize")
	}
}

func TestGameBudsTrayBattery(t *testing.T) {
	tests := []struct {
		name     string
		state    protocol.DeviceState
		expected int
	}{
		{
			name: "Both wearing, left lower",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(50),
				RightBattery: intPtr(75),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both wearing, right lower",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(80),
				RightBattery: intPtr(60),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 60,
		},
		{
			name: "Left in case, right wearing",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(90),
				RightBattery: intPtr(50),
				LeftStatus:   earbudStatusPtr(protocol.StatusInCase),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both in case falls back to available batteries",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(80),
				RightBattery: intPtr(75),
				LeftStatus:   earbudStatusPtr(protocol.StatusInCase),
				RightStatus:  earbudStatusPtr(protocol.StatusInCase),
			},
			expected: 75,
		},
		{
			name: "One out with zero battery",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(0),
				RightBattery: intPtr(50),
				LeftStatus:   earbudStatusPtr(protocol.StatusOut),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: 50,
		},
		{
			name: "Both zero battery",
			state: protocol.DeviceState{
				LeftBattery:  intPtr(0),
				RightBattery: intPtr(0),
				LeftStatus:   earbudStatusPtr(protocol.StatusWorn),
				RightStatus:  earbudStatusPtr(protocol.StatusWorn),
			},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gameBudsTrayBattery(tt.state)
			if got != tt.expected {
				t.Errorf("gameBudsTrayBattery() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestSingleBatteryFormatting(t *testing.T) {
	if got := formatBatteryMenuTitle(intPtr(72)); got != "  🔋 Battery: 72%" {
		t.Errorf("formatBatteryMenuTitle() = %q", got)
	}
	if got := formatBatteryMenuTitle(nil); got != "  Battery: --" {
		t.Errorf("formatBatteryMenuTitle(nil) = %q", got)
	}

	charging := true
	state := protocol.DeviceState{IsCharging: &charging}
	if got := formatHyperXBatteryMenuTitle(state); got != "  ⚡ Battery: Charging" {
		t.Errorf("formatHyperXBatteryMenuTitle(charging) = %q", got)
	}

	if got := formatTrayBattery("🎧", 45, false); got != "🎧45%" {
		t.Errorf("formatTrayBattery(with battery) = %q", got)
	}
	if got := formatTrayBattery("🎧", -1, true); got != "🎧⚡" {
		t.Errorf("formatTrayBattery(charging) = %q", got)
	}
	if got := formatTrayBattery("🎧", -1, false); got != "" {
		t.Errorf("formatTrayBattery(empty) = %q", got)
	}
}

func TestBuildTrayTitleParts_StableOrderAndSkipsWarnings(t *testing.T) {
	hyperxBattery := 79
	memoryPercent := 51
	razerBattery := 33
	devices := map[string]protocol.DeviceState{
		"razer-warning": {
			DeviceType:  protocol.DeviceTypeRazer,
			IsConnected: true,
			Warning:     "Battery unavailable",
		},
		"hyperx": {
			DeviceType:  protocol.DeviceTypeHyperXCloudAlpha,
			IsConnected: true,
			Battery:     &hyperxBattery,
		},
		"razer": {
			DeviceType:  protocol.DeviceTypeRazer,
			IsConnected: true,
			Battery:     &razerBattery,
		},
	}
	dockerState := docker.DockerState{
		Available: true,
		Containers: []docker.ContainerInfo{
			{ID: "abc", Name: "web"},
		},
	}
	systemState := system.State{
		Available:     true,
		MemoryPercent: &memoryPercent,
	}

	got := buildTrayTitleParts(devices, dockerState, systemState)
	expected := []string{"🎧79%", "🖱️33%", "🐳1", "🧠51%"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("buildTrayTitleParts() = %#v, want %#v", got, expected)
	}
}

func TestBuildTrayTooltipParts(t *testing.T) {
	hyperxBattery := 79
	systemCPU := 12
	systemMemory := 51
	devices := map[string]protocol.DeviceState{
		"razer-warning": {
			DeviceType:  protocol.DeviceTypeRazer,
			IsConnected: true,
			Warning:     "Battery unavailable",
		},
		"hyperx": {
			DeviceType:  protocol.DeviceTypeHyperXCloudAlpha,
			IsConnected: true,
			Battery:     &hyperxBattery,
		},
	}
	dockerState := docker.DockerState{
		Available: true,
		Containers: []docker.ContainerInfo{
			{ID: "abc", Name: "web"},
			{ID: "def", Name: "worker"},
		},
	}
	systemState := system.State{
		Available:     true,
		CPUPercent:    &systemCPU,
		MemoryPercent: &systemMemory,
	}

	got := buildTrayTooltipParts(devices, dockerState, systemState)
	expected := []string{
		"HyperX: Battery: 79%",
		"Docker: 2 container(s)",
		"System: CPU: 12%, Memory: 51%",
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("buildTrayTooltipParts() = %#v, want %#v", got, expected)
	}
}

func TestDeviceMenuStateForType_PrefersVisibleState(t *testing.T) {
	visibleBattery := 92
	devices := map[string]protocol.DeviceState{
		"razer-disconnected": {
			DeviceType: protocol.DeviceTypeRazer,
		},
		"razer-connected": {
			DeviceType:  protocol.DeviceTypeRazer,
			IsConnected: true,
			Battery:     &visibleBattery,
		},
	}

	got := deviceMenuStateForType(devices, protocol.DeviceTypeRazer)
	if !got.IsConnected || got.Battery == nil || *got.Battery != visibleBattery {
		t.Fatalf("deviceMenuStateForType() = %#v, want connected Razer state", got)
	}
}

func BenchmarkFormatGameBudsBattery(b *testing.B) {
	battery := 75
	status := protocol.StatusWorn
	for i := 0; i < b.N; i++ {
		formatGameBudsBattery(&battery, &status, "Left")
	}
}

func BenchmarkGetBatteryIcon(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getBatteryIcon(75)
	}
}

func BenchmarkGetANCIcon(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getANCIcon(protocol.ANCActive)
	}
}

func TestJoinNames(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		max      int
		expected string
	}{
		{
			name:     "Empty list",
			names:    []string{},
			max:      3,
			expected: "",
		},
		{
			name:     "Single name",
			names:    []string{"web"},
			max:      3,
			expected: "web",
		},
		{
			name:     "Under limit",
			names:    []string{"web", "db"},
			max:      3,
			expected: "web, db",
		},
		{
			name:     "At limit",
			names:    []string{"web", "db", "redis"},
			max:      3,
			expected: "web, db, redis",
		},
		{
			name:     "Over limit",
			names:    []string{"web", "db", "redis", "worker", "scheduler"},
			max:      3,
			expected: "web, db, redis +2 more",
		},
		{
			name:     "One over limit",
			names:    []string{"web", "db", "redis", "worker"},
			max:      3,
			expected: "web, db, redis +1 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinNames(tt.names, tt.max)
			if got != tt.expected {
				t.Errorf("joinNames(%v, %d) = %q, want %q", tt.names, tt.max, got, tt.expected)
			}
		})
	}
}

func TestNewTrayManager_DockerFields(t *testing.T) {
	manager := NewTrayManager()

	if manager == nil {
		t.Fatal("NewTrayManager returned nil")
	}

	if manager.dockerMenu != nil {
		t.Error("dockerMenu should be nil before Initialize")
	}
	if manager.dockerStatus != nil {
		t.Error("dockerStatus should be nil before Initialize")
	}
	if manager.dockerStopAll != nil {
		t.Error("dockerStopAll should be nil before Initialize")
	}
	if manager.systemMenu != nil {
		t.Error("systemMenu should be nil before Initialize")
	}
	if manager.systemCPU != nil {
		t.Error("systemCPU should be nil before Initialize")
	}
	if manager.systemMemory != nil {
		t.Error("systemMemory should be nil before Initialize")
	}
	if manager.systemTemp != nil {
		t.Error("systemTemp should be nil before Initialize")
	}
	if manager.systemGPU != nil {
		t.Error("systemGPU should be nil before Initialize")
	}
}

func TestTrayConfig_Defaults(t *testing.T) {
	cfg := TrayConfig{}

	if cfg.DisableGameBuds {
		t.Error("DisableGameBuds should default to false")
	}
	if cfg.DisableRazer {
		t.Error("DisableRazer should default to false")
	}
	if cfg.DisableHyperX {
		t.Error("DisableHyperX should default to false")
	}
	if cfg.DisableSystem {
		t.Error("DisableSystem should default to false")
	}
}

func TestTrayManager_DisabledGameBuds_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	leftBattery := 80
	rightBattery := 70
	state := protocol.DeviceState{
		DeviceID:     "steelseries_gamebuds",
		DeviceType:   "steelseries_gamebuds",
		LeftBattery:  &leftBattery,
		RightBattery: &rightBattery,
		IsConnected:  true,
	}

	manager.updateGameBuds(state)

	if manager.gameBudsMenu != nil {
		t.Error("gameBudsMenu should remain nil when disabled")
	}
}

func TestTrayManager_DisabledRazer_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	battery := 70
	state := protocol.DeviceState{
		DeviceID:    "razer-device",
		DeviceType:  protocol.DeviceTypeRazer,
		Battery:     &battery,
		IsConnected: true,
	}

	manager.updateRazer(state)

	if manager.razerMenu != nil {
		t.Error("razerMenu should remain nil when disabled")
	}
}

func TestTrayManager_DisabledHyperX_NilGuard(t *testing.T) {
	manager := NewTrayManager()

	battery := 70
	state := protocol.DeviceState{
		DeviceID:    "hyperx_cloud_alpha_wireless",
		DeviceType:  protocol.DeviceTypeHyperXCloudAlpha,
		Battery:     &battery,
		IsConnected: true,
	}

	manager.updateHyperX(state)

	if manager.hyperxMenu != nil {
		t.Error("hyperxMenu should remain nil when disabled")
	}
}

func TestSystemFormatting(t *testing.T) {
	normal := system.State{
		Available:        true,
		CPUPercent:       intPtr(18),
		CPUPeakPercent:   intPtr(92),
		MemoryPercent:    intPtr(32),
		MemoryUsedBytes:  10 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 32 * 1024 * 1024 * 1024,
		MaxCPUTempC:      floatPtr(64.4),
		MaxGPUTempC:      floatPtr(72.8),
		MaxSystemTempC:   floatPtr(72.8),
		GPUs: []system.GPUStats{
			{Name: "nvidia", TemperatureC: floatPtr(72.8)},
		},
	}

	if got := formatSystemCPUMenuTitle(normal); got != "  ⚙️ CPU: 18%" {
		t.Errorf("formatSystemCPUMenuTitle() = %q", got)
	}
	if got := formatSystemPeakMenuTitle(normal); got != "  🔥 CPU Peak: 92% last 60s" {
		t.Errorf("formatSystemPeakMenuTitle() = %q", got)
	}
	if got := formatSystemMemoryMenuTitle(normal); got != "  🧠 Memory: 32% (10.0 GiB / 32.0 GiB)" {
		t.Errorf("formatSystemMemoryMenuTitle() = %q", got)
	}
	if got := formatSystemTempMenuTitle(normal); got != "  🌡️ Temp: CPU 64°C / GPU 73°C / Max 73°C" {
		t.Errorf("formatSystemTempMenuTitle() = %q", got)
	}
	if got := formatSystemGPUMenuTitle(normal.GPUs); got != "  🎮 GPU: nvidia 73°C" {
		t.Errorf("formatSystemGPUMenuTitle() = %q", got)
	}
	richGPU := system.GPUStats{
		Name:             "NVIDIA RTX",
		TemperatureC:     floatPtr(84),
		UtilizationPct:   intPtr(97),
		MemoryUsedBytes:  uint64Ptr(8 * 1024 * 1024 * 1024),
		MemoryTotalBytes: uint64Ptr(24 * 1024 * 1024 * 1024),
		PowerDrawW:       floatPtr(312.5),
		FanSpeedPct:      intPtr(61),
		GraphicsClockMHz: intPtr(2520),
		MemoryClockMHz:   intPtr(10501),
		PState:           "P2",
	}
	if got := formatGPUCompact(richGPU); got != "NVIDIA RTX 84°C 97% 8.0 GiB/24.0 GiB VRAM 312W fan 61% 2520/10501 MHz P2" {
		t.Errorf("formatGPUCompact() = %q", got)
	}
	if got := formatSystemMemoryTitle(32); got != "🧠32%" {
		t.Errorf("formatSystemMemoryTitle() = %q", got)
	}

	spiking := normal
	spiking.CPUSpiking = true
	if got := formatSystemCPUMenuTitle(spiking); got != "  🔥 CPU: 18%" {
		t.Errorf("formatSystemCPUMenuTitle(spiking) = %q", got)
	}
	if got := formatSystemCPUTitle(87); got != "🔥87%" {
		t.Errorf("formatSystemCPUTitle() = %q", got)
	}
	if got := formatSystemTemperatureTitle(normal); got != "" {
		t.Errorf("formatSystemTemperatureTitle(normal) = %q", got)
	}

	hotGPU := normal
	hotGPU.MaxGPUTempC = floatPtr(84)
	if got := formatSystemTemperatureTitle(hotGPU); got != "" {
		t.Errorf("formatSystemTemperatureTitle(unsustained hotGPU) = %q", got)
	}
	hotGPU.HotGPUTempC = floatPtr(84)
	if got := formatSystemTemperatureTitle(hotGPU); got != "🎮84°C" {
		t.Errorf("formatSystemTemperatureTitle(hotGPU) = %q", got)
	}
}

func TestSystemFormatting_UnknownValues(t *testing.T) {
	state := system.State{Available: true}

	if got := formatSystemCPUMenuTitle(state); got != "  ⚙️ CPU: --" {
		t.Errorf("formatSystemCPUMenuTitle() = %q", got)
	}
	if got := formatSystemPeakMenuTitle(state); got != "  🔥 CPU Peak: --" {
		t.Errorf("formatSystemPeakMenuTitle() = %q", got)
	}
	if got := formatSystemMemoryMenuTitle(state); got != "  🧠 Memory: --" {
		t.Errorf("formatSystemMemoryMenuTitle() = %q", got)
	}
	if got := formatSystemTempMenuTitle(state); got != "  🌡️ Temp: --" {
		t.Errorf("formatSystemTempMenuTitle() = %q", got)
	}
	if got := formatSystemGPUMenuTitle(nil); got != "  🎮 GPU: --" {
		t.Errorf("formatSystemGPUMenuTitle() = %q", got)
	}
	if got := formatSystemTooltip(state); got != "" {
		t.Errorf("formatSystemTooltip() = %q", got)
	}
}

func intPtr(v int) *int {
	return &v
}

func floatPtr(v float64) *float64 {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func earbudStatusPtr(v protocol.EarbudStatus) *protocol.EarbudStatus {
	return &v
}
