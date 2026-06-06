package system

import (
	"fmt"
	"sort"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type nvmlLib interface {
	InitWithFlags(uint32) nvml.Return
	Shutdown() nvml.Return
	DeviceGetCount() (int, nvml.Return)
	DeviceGetHandleByIndex(int) (nvml.Device, nvml.Return)
}

type realNVMLLib struct{}

func (realNVMLLib) InitWithFlags(flags uint32) nvml.Return {
	return nvml.InitWithFlags(flags)
}

func (realNVMLLib) Shutdown() nvml.Return {
	return nvml.Shutdown()
}

func (realNVMLLib) DeviceGetCount() (int, nvml.Return) {
	return nvml.DeviceGetCount()
}

func (realNVMLLib) DeviceGetHandleByIndex(index int) (nvml.Device, nvml.Return) {
	return nvml.DeviceGetHandleByIndex(index)
}

type NVIDIASampler struct {
	mu          sync.Mutex
	lib         nvmlLib
	governor    *GPUGovernor
	initialized bool
	enabled     bool
	initTried   bool
}

func NewNVIDIASampler() *NVIDIASampler {
	return NewNVIDIASamplerWithLib(realNVMLLib{})
}

// NewNVIDIASamplerWithConfig builds a sampler that, when cfg.Enabled, also runs
// the opt-in GPU thermal guard against the live device handles it samples.
func NewNVIDIASamplerWithConfig(cfg GovernorConfig) *NVIDIASampler {
	s := NewNVIDIASamplerWithLib(realNVMLLib{})
	if cfg.Enabled {
		s.governor = NewGPUGovernor(cfg)
	}
	return s
}

func NewNVIDIASamplerWithLib(lib nvmlLib) *NVIDIASampler {
	return &NVIDIASampler{lib: lib}
}

func (s *NVIDIASampler) Sample() ([]GPUStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initTried {
		s.initTried = true
		if err := s.initLocked(); err != nil {
			return nil, nil
		}
	}
	if !s.enabled {
		return nil, nil
	}

	count, ret := s.lib.DeviceGetCount()
	if ret != nvml.SUCCESS || count == 0 {
		s.disableLocked()
		return nil, nil
	}

	gpus := make([]GPUStats, 0, count)
	for i := 0; i < count; i++ {
		device, ret := s.lib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			continue
		}
		stats := collectNVMLGPU(i, device)
		if s.governor != nil {
			s.governor.Reconcile(device, stats)
		}
		gpus = append(gpus, stats)
	}
	if len(gpus) == 0 {
		s.disableLocked()
		return nil, nil
	}
	return gpus, nil
}

func (s *NVIDIASampler) initLocked() error {
	if s.lib == nil {
		return fmt.Errorf("nvml library unavailable")
	}

	ret := s.lib.InitWithFlags(nvml.INIT_FLAG_NO_GPUS)
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("nvml init: %s", nvml.ErrorString(ret))
	}
	s.initialized = ret == nvml.SUCCESS

	count, ret := s.lib.DeviceGetCount()
	if ret != nvml.SUCCESS || count == 0 {
		s.disableLocked()
		return fmt.Errorf("no nvidia gpu detected")
	}

	s.enabled = true
	return nil
}

func (s *NVIDIASampler) disableLocked() {
	s.enabled = false
	if s.initialized {
		s.lib.Shutdown()
		s.initialized = false
	}
}

// Close restores any GPUs the thermal guard clamped and shuts down NVML. It is
// invoked from the monitor's shutdown path so quitting never leaves a card
// throttled.
func (s *NVIDIASampler) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.governor != nil {
		s.governor.RestoreAll()
	}
	s.disableLocked()
	return nil
}

func collectNVMLGPU(index int, device nvml.Device) GPUStats {
	stats := GPUStats{Index: index, Name: fmt.Sprintf("NVIDIA GPU %d", index)}
	if name, ret := device.GetName(); ret == nvml.SUCCESS && name != "" {
		stats.Name = name
	}
	if uuid, ret := device.GetUUID(); ret == nvml.SUCCESS && uuid != "" {
		stats.ID = uuid
	}
	if stats.ID == "" {
		stats.ID = fmt.Sprintf("nvidia-%d", index)
	}

	if utilization, ret := device.GetUtilizationRates(); ret == nvml.SUCCESS {
		stats.UtilizationPct = intPtr(int(utilization.Gpu))
		stats.MemoryUtilizationPct = intPtr(int(utilization.Memory))
	}
	if memory, ret := device.GetMemoryInfo(); ret == nvml.SUCCESS {
		stats.MemoryUsedBytes = uint64Ptr(memory.Used)
		stats.MemoryTotalBytes = uint64Ptr(memory.Total)
	}
	if temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
		stats.TemperatureC = float64Ptr(float64(temp))
	}
	if fan, ret := device.GetFanSpeed(); ret == nvml.SUCCESS {
		stats.FanSpeedPct = intPtr(int(fan))
	}
	if power, ret := device.GetPowerUsage(); ret == nvml.SUCCESS {
		stats.PowerDrawW = milliwattsPtr(power)
	}
	if limit, ret := device.GetPowerManagementLimit(); ret == nvml.SUCCESS {
		stats.PowerLimitW = milliwattsPtr(limit)
	}
	if limit, ret := device.GetPowerManagementDefaultLimit(); ret == nvml.SUCCESS {
		stats.DefaultPowerLimitW = milliwattsPtr(limit)
	}
	if minLimit, maxLimit, ret := device.GetPowerManagementLimitConstraints(); ret == nvml.SUCCESS {
		stats.MinPowerLimitW = milliwattsPtr(minLimit)
		stats.MaxPowerLimitW = milliwattsPtr(maxLimit)
	}
	if clock, ret := device.GetClockInfo(nvml.CLOCK_GRAPHICS); ret == nvml.SUCCESS {
		stats.GraphicsClockMHz = intPtr(int(clock))
	}
	if clock, ret := device.GetClockInfo(nvml.CLOCK_MEM); ret == nvml.SUCCESS {
		stats.MemoryClockMHz = intPtr(int(clock))
	}
	if pstate, ret := device.GetPerformanceState(); ret == nvml.SUCCESS {
		stats.PState = formatPState(pstate)
	}

	stats.Processes = collectNVMLProcesses(device)
	return stats
}

func collectNVMLProcesses(device nvml.Device) []GPUProcess {
	processesByKey := make(map[string]GPUProcess)
	collectProcessKind(processesByKey, "graphics", device.GetGraphicsRunningProcesses)
	collectProcessKind(processesByKey, "compute", device.GetComputeRunningProcesses)

	processes := make([]GPUProcess, 0, len(processesByKey))
	for _, process := range processesByKey {
		processes = append(processes, process)
	}
	sort.Slice(processes, func(i, j int) bool {
		if processes[i].PID == processes[j].PID {
			return processes[i].Kind < processes[j].Kind
		}
		return processes[i].PID < processes[j].PID
	})
	return processes
}

func collectProcessKind(
	processes map[string]GPUProcess,
	kind string,
	read func() ([]nvml.ProcessInfo, nvml.Return),
) {
	infos, ret := read()
	if ret != nvml.SUCCESS {
		return
	}
	for _, info := range infos {
		process := GPUProcess{PID: int(info.Pid), Kind: kind}
		if info.UsedGpuMemory != ^uint64(0) {
			process.UsedMemoryBytes = uint64Ptr(info.UsedGpuMemory)
		}
		processes[fmt.Sprintf("%s:%d", kind, process.PID)] = process
	}
}

func formatPState(pstate nvml.Pstates) string {
	if pstate == nvml.PSTATE_UNKNOWN {
		return "unknown"
	}
	if pstate >= nvml.PSTATE_0 && pstate <= nvml.PSTATE_15 {
		return fmt.Sprintf("P%d", int(pstate))
	}
	return ""
}

func milliwattsPtr(value uint32) *float64 {
	return float64Ptr(float64(value) / 1000)
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}
