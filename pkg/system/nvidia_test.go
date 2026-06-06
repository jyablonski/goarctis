package system

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

func TestNVIDIASampler_DisablesWhenNVMLUnavailable(t *testing.T) {
	lib := &fakeNVMLLib{initRet: nvml.ERROR_LIBRARY_NOT_FOUND}
	sampler := NewNVIDIASamplerWithLib(lib)

	gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(gpus) != 0 {
		t.Fatalf("gpu count = %d, want 0", len(gpus))
	}

	_, _ = sampler.Sample()
	if lib.initCalls != 1 {
		t.Fatalf("init calls = %d, want 1", lib.initCalls)
	}
	if lib.shutdownCalls != 0 {
		t.Fatalf("shutdown calls = %d, want 0", lib.shutdownCalls)
	}
}

func TestNVIDIASampler_DisablesWhenNoDevicesDetected(t *testing.T) {
	lib := &fakeNVMLLib{initRet: nvml.SUCCESS, countRet: nvml.SUCCESS}
	sampler := NewNVIDIASamplerWithLib(lib)

	gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(gpus) != 0 {
		t.Fatalf("gpu count = %d, want 0", len(gpus))
	}
	if lib.shutdownCalls != 1 {
		t.Fatalf("shutdown calls = %d, want 1", lib.shutdownCalls)
	}
}

func TestNVIDIASampler_CollectsOptionalMetrics(t *testing.T) {
	device := &nvmlmock.Device{
		GetNameFunc: func() (string, nvml.Return) {
			return "NVIDIA RTX", nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-123", nvml.SUCCESS
		},
		GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
			return nvml.Utilization{Gpu: 97, Memory: 42}, nvml.SUCCESS
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{Used: 8 * 1024 * 1024 * 1024, Total: 24 * 1024 * 1024 * 1024}, nvml.SUCCESS
		},
		GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
			return 83, nvml.SUCCESS
		},
		GetFanSpeedFunc: func() (uint32, nvml.Return) {
			return 61, nvml.SUCCESS
		},
		GetPowerUsageFunc: func() (uint32, nvml.Return) {
			return 312500, nvml.SUCCESS
		},
		GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
			return 350000, nvml.SUCCESS
		},
		GetPowerManagementDefaultLimitFunc: func() (uint32, nvml.Return) {
			return 400000, nvml.SUCCESS
		},
		GetPowerManagementLimitConstraintsFunc: func() (uint32, uint32, nvml.Return) {
			return 200000, 450000, nvml.SUCCESS
		},
		GetClockInfoFunc: func(clock nvml.ClockType) (uint32, nvml.Return) {
			if clock == nvml.CLOCK_GRAPHICS {
				return 2520, nvml.SUCCESS
			}
			return 10501, nvml.SUCCESS
		},
		GetPerformanceStateFunc: func() (nvml.Pstates, nvml.Return) {
			return nvml.PSTATE_2, nvml.SUCCESS
		},
		GetGraphicsRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			return []nvml.ProcessInfo{{Pid: 100, UsedGpuMemory: 1024}}, nvml.SUCCESS
		},
		GetComputeRunningProcessesFunc: func() ([]nvml.ProcessInfo, nvml.Return) {
			return []nvml.ProcessInfo{{Pid: 200, UsedGpuMemory: 2048}}, nvml.SUCCESS
		},
	}
	lib := &fakeNVMLLib{
		initRet:  nvml.SUCCESS,
		count:    1,
		countRet: nvml.SUCCESS,
		devices:  []nvml.Device{device},
	}
	sampler := NewNVIDIASamplerWithLib(lib)

	gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("gpu count = %d, want 1", len(gpus))
	}

	gpu := gpus[0]
	if gpu.ID != "GPU-123" || gpu.Name != "NVIDIA RTX" {
		t.Fatalf("gpu identity = %q/%q, want GPU-123/NVIDIA RTX", gpu.ID, gpu.Name)
	}
	if gpu.UtilizationPct == nil || *gpu.UtilizationPct != 97 {
		t.Fatalf("utilization = %v, want 97", gpu.UtilizationPct)
	}
	if gpu.TemperatureC == nil || *gpu.TemperatureC != 83 {
		t.Fatalf("temperature = %v, want 83", gpu.TemperatureC)
	}
	if gpu.PowerDrawW == nil || *gpu.PowerDrawW != 312.5 {
		t.Fatalf("power draw = %v, want 312.5", gpu.PowerDrawW)
	}
	if gpu.GraphicsClockMHz == nil || *gpu.GraphicsClockMHz != 2520 {
		t.Fatalf("graphics clock = %v, want 2520", gpu.GraphicsClockMHz)
	}
	if gpu.MemoryClockMHz == nil || *gpu.MemoryClockMHz != 10501 {
		t.Fatalf("memory clock = %v, want 10501", gpu.MemoryClockMHz)
	}
	if gpu.PState != "P2" {
		t.Fatalf("pstate = %q, want P2", gpu.PState)
	}
	if len(gpu.Processes) != 2 {
		t.Fatalf("process count = %d, want 2", len(gpu.Processes))
	}
}

type fakeNVMLLib struct {
	initRet       nvml.Return
	count         int
	countRet      nvml.Return
	devices       []nvml.Device
	initCalls     int
	shutdownCalls int
}

func (l *fakeNVMLLib) InitWithFlags(uint32) nvml.Return {
	l.initCalls++
	return l.initRet
}

func (l *fakeNVMLLib) Shutdown() nvml.Return {
	l.shutdownCalls++
	return nvml.SUCCESS
}

func (l *fakeNVMLLib) DeviceGetCount() (int, nvml.Return) {
	return l.count, l.countRet
}

func (l *fakeNVMLLib) DeviceGetHandleByIndex(index int) (nvml.Device, nvml.Return) {
	if index < 0 || index >= len(l.devices) {
		return nil, nvml.ERROR_NOT_FOUND
	}
	return l.devices[index], nvml.SUCCESS
}
