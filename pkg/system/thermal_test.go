package system

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

func TestDefaultThermalSampler_UsesNVMLForNVIDIAGPUs(t *testing.T) {
	hwmon := &fakeThermalSampler{
		sensors: []SensorReading{{Chip: "amdgpu", TemperatureC: 65}},
		gpus: []GPUStats{
			{ID: "/sys/class/hwmon/hwmon0", Name: "nvidia", TemperatureC: float64Ptr(70)},
			{ID: "/sys/class/hwmon/hwmon1", Name: "amdgpu", TemperatureC: float64Ptr(65)},
		},
	}
	nvidia := NewNVIDIASamplerWithLib(&fakeNVMLLib{
		initRet:  nvml.SUCCESS,
		count:    1,
		countRet: nvml.SUCCESS,
		devices: []nvml.Device{&nvmlmock.Device{
			GetNameFunc: func() (string, nvml.Return) {
				return "NVIDIA RTX", nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-123", nvml.SUCCESS
			},
			GetTemperatureFunc: func(nvml.TemperatureSensors) (uint32, nvml.Return) {
				return 82, nvml.SUCCESS
			},
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.ERROR_NOT_SUPPORTED
			},
			GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
				return nvml.Memory{}, nvml.ERROR_NOT_SUPPORTED
			},
			GetFanSpeedFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerManagementDefaultLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerManagementLimitConstraintsFunc: func() (uint32, uint32, nvml.Return) {
				return 0, 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetClockInfoFunc: func(nvml.ClockType) (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPerformanceStateFunc: func() (nvml.Pstates, nvml.Return) {
				return nvml.PSTATE_UNKNOWN, nvml.ERROR_NOT_SUPPORTED
			},
			GetGraphicsRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_NOT_SUPPORTED
			},
			GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
				return nil, nvml.ERROR_NOT_SUPPORTED
			},
		}},
	})
	sampler := NewDefaultThermalSamplerWithProviders(hwmon, nvidia)

	_, gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("gpu count = %d, want 2", len(gpus))
	}
	if gpus[0].Name != "amdgpu" {
		t.Fatalf("first gpu name = %q, want amdgpu", gpus[0].Name)
	}
	if gpus[1].Name != "NVIDIA RTX" {
		t.Fatalf("second gpu name = %q, want NVIDIA RTX", gpus[1].Name)
	}
}

type fakeThermalSampler struct {
	sensors []SensorReading
	gpus    []GPUStats
	err     error
}

func (s *fakeThermalSampler) Sample() ([]SensorReading, []GPUStats, error) {
	return s.sensors, s.gpus, s.err
}
