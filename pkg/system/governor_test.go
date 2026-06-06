package system

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// limitRecorder is an nvml.Device whose SetPowerManagementLimit records every
// requested value and can be forced to return a specific nvml.Return.
type limitRecorder struct {
	*nvmlmock.Device
	sets []uint32
	ret  nvml.Return
}

func newLimitRecorder() *limitRecorder {
	r := &limitRecorder{Device: &nvmlmock.Device{}, ret: nvml.SUCCESS}
	r.SetPowerManagementLimitFunc = func(limit uint32) nvml.Return {
		r.sets = append(r.sets, limit)
		return r.ret
	}
	return r
}

func enabledConfig() GovernorConfig {
	return GovernorConfig{
		Enabled:        true,
		ClampTempC:     DefaultClampTempC,
		ReleaseTempC:   DefaultReleaseTempC,
		TargetFraction: DefaultTargetFraction,
	}
}

func gpuAt(temp float64) GPUStats {
	return GPUStats{
		ID:                 "GPU-abc",
		TemperatureC:       float64Ptr(temp),
		DefaultPowerLimitW: float64Ptr(300),
		MinPowerLimitW:     float64Ptr(150),
		MaxPowerLimitW:     float64Ptr(350),
	}
}

func TestGovernor_ClampsOnceAboveThresholdThenHolds(t *testing.T) {
	g := NewGPUGovernor(enabledConfig())
	device := newLimitRecorder()

	g.Reconcile(device, gpuAt(86)) // crosses 85 -> clamp to 300*0.80 = 240W
	g.Reconcile(device, gpuAt(88)) // still hot, already clamped -> no new write

	if len(device.sets) != 1 {
		t.Fatalf("set calls = %d, want 1", len(device.sets))
	}
	if device.sets[0] != 240_000 {
		t.Fatalf("clamp limit = %d mW, want 240000", device.sets[0])
	}
}

func TestGovernor_HoldsInBand(t *testing.T) {
	g := NewGPUGovernor(enabledConfig())
	device := newLimitRecorder()

	g.Reconcile(device, gpuAt(80)) // between release(75) and clamp(85)

	if len(device.sets) != 0 {
		t.Fatalf("set calls = %d, want 0", len(device.sets))
	}
}

func TestGovernor_RestoresOnCooldown(t *testing.T) {
	g := NewGPUGovernor(enabledConfig())
	device := newLimitRecorder()

	g.Reconcile(device, gpuAt(90)) // clamp -> 240W
	g.Reconcile(device, gpuAt(74)) // at/below release -> restore default 300W

	if len(device.sets) != 2 {
		t.Fatalf("set calls = %d, want 2", len(device.sets))
	}
	if device.sets[1] != 300_000 {
		t.Fatalf("restore limit = %d mW, want 300000", device.sets[1])
	}
	if len(g.clamped) != 0 {
		t.Fatalf("clamped records = %d, want 0", len(g.clamped))
	}
}

func TestGovernor_ClampRespectsMinConstraint(t *testing.T) {
	cfg := enabledConfig()
	cfg.TargetFraction = 0.10 // 300*0.10 = 30W, below the 150W floor
	g := NewGPUGovernor(cfg)
	device := newLimitRecorder()

	g.Reconcile(device, gpuAt(95))

	if len(device.sets) != 1 || device.sets[0] != 150_000 {
		t.Fatalf("clamp limit = %v, want [150000]", device.sets)
	}
}

func TestGovernor_RestoreAllUnclampsEveryGPU(t *testing.T) {
	g := NewGPUGovernor(enabledConfig())
	deviceA := newLimitRecorder()
	deviceB := newLimitRecorder()

	a := gpuAt(90)
	a.ID = "GPU-a"
	b := gpuAt(90)
	b.ID = "GPU-b"

	g.Reconcile(deviceA, a)
	g.Reconcile(deviceB, b)
	g.RestoreAll()

	if len(deviceA.sets) != 2 || deviceA.sets[1] != 300_000 {
		t.Fatalf("device A sets = %v, want clamp then restore 300000", deviceA.sets)
	}
	if len(deviceB.sets) != 2 || deviceB.sets[1] != 300_000 {
		t.Fatalf("device B sets = %v, want clamp then restore 300000", deviceB.sets)
	}
	if len(g.clamped) != 0 {
		t.Fatalf("clamped records = %d, want 0", len(g.clamped))
	}
}

func TestGovernor_SelfDisablesWithoutPermission(t *testing.T) {
	g := NewGPUGovernor(enabledConfig())
	device := newLimitRecorder()
	device.ret = nvml.ERROR_NO_PERMISSION

	g.Reconcile(device, gpuAt(90)) // attempted clamp fails
	g.Reconcile(device, gpuAt(95)) // guard disabled -> no further attempts

	if len(device.sets) != 1 {
		t.Fatalf("set calls = %d, want 1 (then disabled)", len(device.sets))
	}
	if !g.disabled {
		t.Fatalf("governor should be disabled after permission error")
	}
	if len(g.clamped) != 0 {
		t.Fatalf("clamped records = %d, want 0 (clamp failed)", len(g.clamped))
	}
}

func TestGovernor_InvalidConfigStartsDisabled(t *testing.T) {
	cases := map[string]GovernorConfig{
		"release above clamp": {Enabled: true, ClampTempC: 70, ReleaseTempC: 80, TargetFraction: 0.8},
		"fraction over one":   {Enabled: true, ClampTempC: 85, ReleaseTempC: 75, TargetFraction: 1.5},
		"fraction zero":       {Enabled: true, ClampTempC: 85, ReleaseTempC: 75, TargetFraction: 0},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			g := NewGPUGovernor(cfg)
			device := newLimitRecorder()
			g.Reconcile(device, gpuAt(95))
			if len(device.sets) != 0 {
				t.Fatalf("set calls = %d, want 0 for invalid config", len(device.sets))
			}
		})
	}
}

func TestGovernor_DisabledConfigNeverActuates(t *testing.T) {
	g := NewGPUGovernor(GovernorConfig{}) // Enabled: false
	device := newLimitRecorder()

	g.Reconcile(device, gpuAt(99))

	if len(device.sets) != 0 {
		t.Fatalf("set calls = %d, want 0 when disabled", len(device.sets))
	}
}
