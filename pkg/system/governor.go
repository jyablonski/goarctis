package system

import (
	"log"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Default thermal-guard tuning, also used for the CLI flag defaults.
// See docs/gpu_thermal_guard.md.
const (
	DefaultClampTempC     = 85.0
	DefaultReleaseTempC   = 75.0
	DefaultTargetFraction = 0.80
)

// GovernorConfig configures the opt-in GPU thermal guard. When Enabled, the
// guard reduces an NVIDIA GPU's power limit once it reaches ClampTempC and
// restores the card's default limit once it cools to ReleaseTempC.
//
// The thresholds are currently fixed to the package defaults; the struct keeps
// them as fields so per-value tuning (e.g. CLI flags) can be layered on later
// without reworking the governor.
type GovernorConfig struct {
	Enabled        bool
	ClampTempC     float64
	ReleaseTempC   float64
	TargetFraction float64
}

// DefaultGovernorConfig returns the thermal-guard config with the standard
// thresholds, toggled by enabled. This is the single knob exposed to the CLI.
func DefaultGovernorConfig(enabled bool) GovernorConfig {
	return GovernorConfig{
		Enabled:        enabled,
		ClampTempC:     DefaultClampTempC,
		ReleaseTempC:   DefaultReleaseTempC,
		TargetFraction: DefaultTargetFraction,
	}
}

// valid reports whether the tuning values are usable. The release threshold
// must sit below the clamp threshold (hysteresis gap) and the target fraction
// must reduce the limit without exceeding it.
func (c GovernorConfig) valid() bool {
	return c.ReleaseTempC < c.ClampTempC &&
		c.TargetFraction > 0 && c.TargetFraction <= 1
}

type clampRecord struct {
	device    nvml.Device // valid until our nvml.Shutdown — used for restore
	defaultMW uint32
}

// GPUGovernor applies a single, bounded, reversible power-limit clamp per GPU
// based on temperature. It is safe for concurrent use; Reconcile is called once
// per GPU per sampling poll.
type GPUGovernor struct {
	mu       sync.Mutex
	cfg      GovernorConfig
	clamped  map[string]clampRecord // keyed by GPU UUID
	disabled bool                   // tripped on permission/not-supported, or invalid config
}

// NewGPUGovernor builds a governor. An enabled-but-invalid config is accepted
// but starts disabled, so the app degrades to pure monitoring instead of
// actuating on garbage values.
func NewGPUGovernor(cfg GovernorConfig) *GPUGovernor {
	g := &GPUGovernor{cfg: cfg, clamped: make(map[string]clampRecord)}
	if cfg.Enabled && !cfg.valid() {
		log.Printf("gpu thermal guard: invalid config (clamp=%.0f release=%.0f frac=%.2f); disabling",
			cfg.ClampTempC, cfg.ReleaseTempC, cfg.TargetFraction)
		g.disabled = true
	}
	return g
}

// Reconcile evaluates a single GPU's temperature against the configured
// thresholds and clamps or restores its power limit as needed. In-band
// temperatures (release < temp < clamp) hold the current state.
func (g *GPUGovernor) Reconcile(device nvml.Device, stats GPUStats) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.disabled || !g.cfg.Enabled || stats.ID == "" ||
		stats.TemperatureC == nil || stats.DefaultPowerLimitW == nil {
		return
	}

	temp := *stats.TemperatureC
	_, clamped := g.clamped[stats.ID]
	switch {
	case !clamped && temp >= g.cfg.ClampTempC:
		g.clampLocked(device, stats)
	case clamped && temp <= g.cfg.ReleaseTempC:
		g.restoreLocked(stats.ID)
	}
}

// RestoreAll returns every clamped GPU to its default power limit. It is called
// on shutdown so quitting never leaves a card throttled.
func (g *GPUGovernor) RestoreAll() {
	g.mu.Lock()
	defer g.mu.Unlock()

	ids := make([]string, 0, len(g.clamped))
	for id := range g.clamped {
		ids = append(ids, id)
	}
	for _, id := range ids {
		g.restoreLocked(id)
	}
}

func (g *GPUGovernor) clampLocked(device nvml.Device, stats GPUStats) {
	defaultW := *stats.DefaultPowerLimitW
	targetW := defaultW * g.cfg.TargetFraction
	if stats.MinPowerLimitW != nil && targetW < *stats.MinPowerLimitW {
		targetW = *stats.MinPowerLimitW
	}
	if stats.MaxPowerLimitW != nil && targetW > *stats.MaxPowerLimitW {
		targetW = *stats.MaxPowerLimitW
	}

	targetMW := wattsToMilliwatts(targetW)
	if targetMW == 0 {
		return
	}
	if ret := device.SetPowerManagementLimit(targetMW); ret != nvml.SUCCESS {
		g.handleSetFailureLocked("clamp", stats.ID, ret)
		return
	}

	g.clamped[stats.ID] = clampRecord{
		device:    device,
		defaultMW: wattsToMilliwatts(defaultW),
	}
	log.Printf("gpu thermal guard: %s reached %.0f°C, power limit %.0fW -> %.0fW",
		stats.ID, *stats.TemperatureC, defaultW, targetW)
}

func (g *GPUGovernor) restoreLocked(id string) {
	record, ok := g.clamped[id]
	if !ok {
		return
	}
	if ret := record.device.SetPowerManagementLimit(record.defaultMW); ret != nvml.SUCCESS {
		// Keep the record so RestoreAll can retry on shutdown.
		g.handleSetFailureLocked("restore", id, ret)
		return
	}
	delete(g.clamped, id)
	log.Printf("gpu thermal guard: %s cooled down, power limit restored to %.0fW",
		id, float64(record.defaultMW)/1000)
}

func (g *GPUGovernor) handleSetFailureLocked(action, id string, ret nvml.Return) {
	if ret == nvml.ERROR_NO_PERMISSION || ret == nvml.ERROR_NOT_SUPPORTED {
		log.Printf("gpu thermal guard: cannot %s power limit for %s (%s); disabling guard (needs root)",
			action, id, nvml.ErrorString(ret))
		g.disabled = true
		return
	}
	log.Printf("gpu thermal guard: %s power limit for %s failed: %s",
		action, id, nvml.ErrorString(ret))
}

// wattsToMilliwatts rounds watts to the milliwatt units NVML expects.
func wattsToMilliwatts(w float64) uint32 {
	if w <= 0 {
		return 0
	}
	return uint32(w*1000 + 0.5)
}
