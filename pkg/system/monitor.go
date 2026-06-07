package system

import (
	"io"
	"log"
	"sync"
	"time"
)

const (
	DefaultPollInterval      = 2 * time.Second
	DefaultCPUSpikeThreshold = 80
	DefaultCPUSpikeHold      = 10 * time.Second
	DefaultCPUPeakWindow     = 60 * time.Second
	DefaultHotTempThresholdC = 80
	DefaultHotTempSustain    = 10 * time.Second
)

const (
	minCPUChange    = 5
	minMemoryChange = 1
	minTempChange   = 1
)

type State struct {
	Available        bool
	CPUPercent       *int
	CPUPeakPercent   *int
	CPUSpiking       bool
	MemoryPercent    *int
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
	GPUs             []GPUStats
	Sensors          []SensorReading
	MaxCPUTempC      *float64
	MaxGPUTempC      *float64
	MaxSystemTempC   *float64
	HotCPUTempC      *float64
	HotGPUTempC      *float64
	HotSystemTempC   *float64
}

type GPUStats struct {
	Index                int
	ID                   string
	Name                 string
	UtilizationPct       *int
	MemoryUtilizationPct *int
	MemoryUsedBytes      *uint64
	MemoryTotalBytes     *uint64
	TemperatureC         *float64
	FanSpeedPct          *int
	PowerDrawW           *float64
	PowerLimitW          *float64
	DefaultPowerLimitW   *float64
	MinPowerLimitW       *float64
	MaxPowerLimitW       *float64
	GraphicsClockMHz     *int
	MemoryClockMHz       *int
	PState               string
	Processes            []GPUProcess
}

type GPUProcess struct {
	PID             int
	Kind            string
	UsedMemoryBytes *uint64
}

type SensorReading struct {
	ID           string
	Label        string
	Source       string
	Chip         string
	TemperatureC float64
	Hidden       bool
}

type Sampler interface {
	Sample() (State, error)
}

type cpuSample struct {
	at      time.Time
	percent int
}

type hotTempTracker struct {
	since time.Time
}

type Monitor struct {
	mu       sync.RWMutex
	state    State
	onChange func(State)
	sampler  Sampler

	lastNotified State
	hasNotified  bool

	pollInterval time.Duration
	stopCh       chan struct{}
	stopped      bool

	now               func() time.Time
	cpuPeakWindow     time.Duration
	cpuSpikeHold      time.Duration
	cpuSpikeThreshold int
	cpuSamples        []cpuSample
	lastSpikeAt       time.Time
	hotTempThresholdC float64
	hotTempSustain    time.Duration
	hotCPUTemp        hotTempTracker
	hotGPUTemp        hotTempTracker
	hotSystemTemp     hotTempTracker
}

func NewMonitor(interval time.Duration) *Monitor {
	return NewMonitorWithSampler(interval, NewProcSampler())
}

// NewMonitorWithConfig builds a monitor whose sampler has the GPU thermal guard
// configured. A disabled config behaves like NewMonitor.
func NewMonitorWithConfig(interval time.Duration, cfg GovernorConfig) *Monitor {
	return NewMonitorWithSampler(interval, NewProcSamplerWithConfig(cfg))
}

func NewMonitorWithSampler(interval time.Duration, sampler Sampler) *Monitor {
	return &Monitor{
		sampler:           sampler,
		pollInterval:      interval,
		stopCh:            make(chan struct{}),
		now:               time.Now,
		cpuPeakWindow:     DefaultCPUPeakWindow,
		cpuSpikeHold:      DefaultCPUSpikeHold,
		cpuSpikeThreshold: DefaultCPUSpikeThreshold,
		hotTempThresholdC: DefaultHotTempThresholdC,
		hotTempSustain:    DefaultHotTempSustain,
	}
}

func (m *Monitor) SetOnChange(fn func(State)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Monitor) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Monitor) Start() {
	m.poll()

	go func() {
		ticker := time.NewTicker(m.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.poll()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	close(m.stopCh)
	sampler := m.sampler
	m.mu.Unlock()

	// Release sampler-held resources (e.g. NVML + thermal-guard restore) once
	// the poll loop has been signalled to stop.
	if closer, ok := sampler.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Printf("system sampler close: %v", err)
		}
	}
}

func (m *Monitor) poll() {
	newState, err := m.sampler.Sample()
	if err != nil {
		log.Printf("System metrics not available: %v", err)
		newState = State{Available: false}
	}

	m.mu.Lock()
	m.decorateState(&newState)
	changed := m.stateChanged(newState)
	m.state = newState
	onChange := m.onChange
	if changed && onChange != nil {
		m.lastNotified = newState
		m.hasNotified = true
	}
	m.mu.Unlock()

	if changed && onChange != nil {
		onChange(newState)
	}
}

func (m *Monitor) decorateState(state *State) {
	if !state.Available {
		m.cpuSamples = nil
		m.lastSpikeAt = time.Time{}
		m.resetHotTemps()
		return
	}

	now := m.now()
	if state.CPUPercent != nil {
		percent := *state.CPUPercent
		m.cpuSamples = append(m.cpuSamples, cpuSample{at: now, percent: percent})
		if percent >= m.cpuSpikeThreshold {
			m.lastSpikeAt = now
		}
	}

	m.pruneCPUSamples(now)
	if peak, ok := m.cpuPeak(); ok {
		state.CPUPeakPercent = intPtr(peak)
	}

	if !m.lastSpikeAt.IsZero() && now.Sub(m.lastSpikeAt) <= m.cpuSpikeHold {
		state.CPUSpiking = true
	}

	m.decorateHotTemps(state, now)
}

func (m *Monitor) decorateHotTemps(state *State, now time.Time) {
	state.HotCPUTempC = m.sustainedHotTemp(&m.hotCPUTemp, state.MaxCPUTempC, now)
	state.HotGPUTempC = m.sustainedHotTemp(&m.hotGPUTemp, state.MaxGPUTempC, now)
	state.HotSystemTempC = m.sustainedHotTemp(&m.hotSystemTemp, state.MaxSystemTempC, now)
}

func (m *Monitor) sustainedHotTemp(tracker *hotTempTracker, temp *float64, now time.Time) *float64 {
	if temp == nil || *temp < m.hotTempThresholdC {
		tracker.since = time.Time{}
		return nil
	}

	if tracker.since.IsZero() {
		tracker.since = now
	}
	if now.Sub(tracker.since) < m.hotTempSustain {
		return nil
	}
	return temp
}

func (m *Monitor) resetHotTemps() {
	m.hotCPUTemp = hotTempTracker{}
	m.hotGPUTemp = hotTempTracker{}
	m.hotSystemTemp = hotTempTracker{}
}

func (m *Monitor) pruneCPUSamples(now time.Time) {
	cutoff := now.Add(-m.cpuPeakWindow)
	keepFrom := 0
	for keepFrom < len(m.cpuSamples) && m.cpuSamples[keepFrom].at.Before(cutoff) {
		keepFrom++
	}
	if keepFrom > 0 {
		m.cpuSamples = append([]cpuSample(nil), m.cpuSamples[keepFrom:]...)
	}
}

func (m *Monitor) cpuPeak() (int, bool) {
	if len(m.cpuSamples) == 0 {
		return 0, false
	}

	peak := m.cpuSamples[0].percent
	for _, sample := range m.cpuSamples[1:] {
		if sample.percent > peak {
			peak = sample.percent
		}
	}
	return peak, true
}

// stateChanged returns true if a new state should be pushed to the UI.
// Must be called with m.mu held.
func (m *Monitor) stateChanged(newState State) bool {
	if !m.hasNotified {
		return true
	}

	old := m.lastNotified
	if old.Available != newState.Available {
		return true
	}
	if old.CPUSpiking != newState.CPUSpiking {
		return true
	}
	if intPointerChanged(old.CPUPercent, newState.CPUPercent, minCPUChange) {
		return true
	}
	if intPointerChanged(old.CPUPeakPercent, newState.CPUPeakPercent, minCPUChange) {
		return true
	}
	if intPointerChanged(old.MemoryPercent, newState.MemoryPercent, minMemoryChange) {
		return true
	}
	if old.MemoryTotalBytes != newState.MemoryTotalBytes {
		return true
	}
	if floatPointerChanged(old.MaxCPUTempC, newState.MaxCPUTempC, minTempChange) {
		return true
	}
	if floatPointerChanged(old.MaxGPUTempC, newState.MaxGPUTempC, minTempChange) {
		return true
	}
	if floatPointerChanged(old.MaxSystemTempC, newState.MaxSystemTempC, minTempChange) {
		return true
	}
	if floatPointerChanged(old.HotCPUTempC, newState.HotCPUTempC, minTempChange) {
		return true
	}
	if floatPointerChanged(old.HotGPUTempC, newState.HotGPUTempC, minTempChange) {
		return true
	}
	if floatPointerChanged(old.HotSystemTempC, newState.HotSystemTempC, minTempChange) {
		return true
	}
	if gpuSummariesChanged(old.GPUs, newState.GPUs) {
		return true
	}
	if len(old.Sensors) != len(newState.Sensors) {
		return true
	}
	return false
}

func intPointerChanged(old, new *int, threshold int) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil || new == nil {
		return true
	}
	return absInt(*old-*new) >= threshold
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func floatPointerChanged(old, new *float64, threshold float64) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil || new == nil {
		return true
	}
	return absFloat(*old-*new) >= threshold
}

func gpuSummariesChanged(old, new []GPUStats) bool {
	if len(old) != len(new) {
		return true
	}
	for i := range old {
		if old[i].ID != new[i].ID || old[i].Name != new[i].Name {
			return true
		}
		if floatPointerChanged(old[i].TemperatureC, new[i].TemperatureC, minTempChange) {
			return true
		}
	}
	return false
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
