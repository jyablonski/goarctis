package system

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var (
	ErrMemInfoMissingTotal = errors.New("meminfo missing MemTotal")
	ErrStatCPULineTooShort = errors.New("stat cpu line too short")
	ErrStatMissingCPU      = errors.New("stat missing aggregate cpu line")
)

type fileReader interface {
	ReadFile(name string) ([]byte, error)
}

type osFileReader struct{}

func (r osFileReader) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

type ThermalSampler interface {
	Sample() ([]SensorReading, []GPUStats, error)
}

type cpuTimes struct {
	idle  uint64
	total uint64
}

// ProcSampler reads host metrics from Linux procfs. CPU percentage is reported
// after the second sample because /proc/stat exposes cumulative counters.
type ProcSampler struct {
	reader  fileReader
	thermal ThermalSampler
	prev    *cpuTimes
}

func NewProcSampler() *ProcSampler {
	return NewProcSamplerWithThermal(osFileReader{}, NewDefaultThermalSampler())
}

// NewProcSamplerWithConfig builds the default sampler with the GPU thermal
// guard configured. A disabled config behaves like NewProcSampler.
func NewProcSamplerWithConfig(cfg GovernorConfig) *ProcSampler {
	return NewProcSamplerWithThermal(osFileReader{}, NewDefaultThermalSamplerWithConfig(cfg))
}

func NewProcSamplerWithReader(reader fileReader) *ProcSampler {
	return &ProcSampler{reader: reader}
}

func NewProcSamplerWithThermal(reader fileReader, thermal ThermalSampler) *ProcSampler {
	return &ProcSampler{reader: reader, thermal: thermal}
}

func (s *ProcSampler) Sample() (State, error) {
	memBytes, err := s.reader.ReadFile("/proc/meminfo")
	if err != nil {
		return State{Available: false}, fmt.Errorf("read meminfo: %w", err)
	}
	usedBytes, totalBytes, memPercent, err := parseMemInfo(string(memBytes))
	if err != nil {
		return State{Available: false}, err
	}

	statBytes, err := s.reader.ReadFile("/proc/stat")
	if err != nil {
		return State{Available: false}, fmt.Errorf("read stat: %w", err)
	}
	current, err := parseCPUTimes(string(statBytes))
	if err != nil {
		return State{Available: false}, err
	}

	state := State{
		Available:        true,
		MemoryPercent:    intPtr(memPercent),
		MemoryUsedBytes:  usedBytes,
		MemoryTotalBytes: totalBytes,
	}

	if s.prev != nil {
		if cpuPercent, ok := calculateCPUPercent(*s.prev, current); ok {
			state.CPUPercent = intPtr(cpuPercent)
		}
	}
	s.prev = &current

	if s.thermal != nil {
		sensors, gpus, err := s.thermal.Sample()
		if err == nil {
			state.Sensors = sensors
			state.GPUs = gpus
			state.MaxCPUTempC = maxCPUTemp(sensors)
			state.MaxGPUTempC = maxGPUTemp(sensors, gpus)
			state.MaxSystemTempC = maxSystemTemp(sensors)
		}
	}

	return state, nil
}

// Close releases the underlying thermal sampler if it holds resources (e.g. the
// NVIDIA sampler's NVML session and any thermal-guard clamps).
func (s *ProcSampler) Close() error {
	if closer, ok := s.thermal.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func parseMemInfo(data string) (usedBytes, totalBytes uint64, percent int, err error) {
	values := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("parse meminfo %s: %w", key, parseErr)
		}
		values[key] = value * 1024
	}

	total, ok := values["MemTotal"]
	if !ok || total == 0 {
		return 0, 0, 0, ErrMemInfoMissingTotal
	}

	available, ok := values["MemAvailable"]
	if !ok {
		available = values["MemFree"] + values["Buffers"] + values["Cached"] + values["SReclaimable"]
		if shmem := values["Shmem"]; shmem <= available {
			available -= shmem
		}
	}
	if available > total {
		available = total
	}

	used := total - available
	memPercent := percentOf(used, total)
	return used, total, memPercent, nil
}

func parseCPUTimes(data string) (cpuTimes, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 5 {
			return cpuTimes{}, ErrStatCPULineTooShort
		}

		var values []uint64
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuTimes{}, fmt.Errorf("parse stat cpu value: %w", err)
			}
			values = append(values, value)
		}

		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}

		var total uint64
		for _, value := range values {
			total += value
		}
		return cpuTimes{idle: idle, total: total}, nil
	}

	return cpuTimes{}, ErrStatMissingCPU
}

func calculateCPUPercent(prev, current cpuTimes) (int, bool) {
	if current.total <= prev.total || current.idle < prev.idle {
		return 0, false
	}

	totalDelta := current.total - prev.total
	if totalDelta == 0 {
		return 0, false
	}

	idleDelta := current.idle - prev.idle
	if idleDelta > totalDelta {
		return 0, false
	}

	busyDelta := totalDelta - idleDelta
	return percentOf(busyDelta, totalDelta), true
}

func percentOf(part, total uint64) int {
	if total == 0 {
		return 0
	}
	value := int((part*100 + total/2) / total)
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func intPtr(v int) *int {
	return &v
}
