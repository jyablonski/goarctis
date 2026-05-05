package system

import (
	"log"
	"sync"
	"time"
)

const (
	DefaultPollInterval      = 2 * time.Second
	DefaultCPUSpikeThreshold = 80
	DefaultCPUSpikeHold      = 10 * time.Second
	DefaultCPUPeakWindow     = 60 * time.Second
)

const (
	minCPUChange    = 5
	minMemoryChange = 1
)

type State struct {
	Available        bool
	CPUPercent       *int
	CPUPeakPercent   *int
	CPUSpiking       bool
	MemoryPercent    *int
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
}

type Sampler interface {
	Sample() (State, error)
}

type cpuSample struct {
	at      time.Time
	percent int
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
}

func NewMonitor(interval time.Duration) *Monitor {
	return NewMonitorWithSampler(interval, NewProcSampler())
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
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
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
