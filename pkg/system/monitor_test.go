package system

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	m := NewMonitor(5 * time.Second)
	if m == nil {
		t.Fatal("NewMonitor returned nil")
	}
	if m.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", m.pollInterval)
	}
	if m.sampler == nil {
		t.Error("sampler should not be nil")
	}
}

func TestMonitor_OnChangeUsesThresholds(t *testing.T) {
	sampler := &fakeSystemSampler{states: []State{
		stateWithMetrics(nil, intPtr(32)),
		stateWithMetrics(intPtr(10), intPtr(32)),
		stateWithMetrics(intPtr(12), intPtr(32)),
		stateWithMetrics(intPtr(16), intPtr(32)),
		stateWithMetrics(intPtr(16), intPtr(33)),
	}}
	m := NewMonitorWithSampler(time.Second, sampler)

	var states []State
	m.SetOnChange(func(state State) {
		states = append(states, state)
	})

	m.poll()
	m.poll()
	m.poll()
	m.poll()
	m.poll()

	if len(states) != 4 {
		t.Fatalf("callback count = %d, want 4", len(states))
	}
	if states[0].MemoryPercent == nil || *states[0].MemoryPercent != 32 {
		t.Fatalf("first memory percent = %v, want 32", states[0].MemoryPercent)
	}
	if states[1].CPUPercent == nil || *states[1].CPUPercent != 10 {
		t.Fatalf("second CPU percent = %v, want 10", states[1].CPUPercent)
	}
	if states[2].CPUPercent == nil || *states[2].CPUPercent != 16 {
		t.Fatalf("third CPU percent = %v, want 16", states[2].CPUPercent)
	}
	if states[3].MemoryPercent == nil || *states[3].MemoryPercent != 33 {
		t.Fatalf("fourth memory percent = %v, want 33", states[3].MemoryPercent)
	}
}

func TestMonitor_OnChangeUsesTemperatureThreshold(t *testing.T) {
	first := stateWithMetrics(intPtr(10), intPtr(32))
	first.MaxSystemTempC = floatPtr(70)
	second := stateWithMetrics(intPtr(10), intPtr(32))
	second.MaxSystemTempC = floatPtr(70.5)
	third := stateWithMetrics(intPtr(10), intPtr(32))
	third.MaxSystemTempC = floatPtr(71)
	sampler := &fakeSystemSampler{states: []State{first, second, third}}
	m := NewMonitorWithSampler(time.Second, sampler)

	var states []State
	m.SetOnChange(func(state State) {
		states = append(states, state)
	})

	m.poll()
	m.poll()
	m.poll()

	if len(states) != 2 {
		t.Fatalf("callback count = %d, want 2", len(states))
	}
	if states[1].MaxSystemTempC == nil || *states[1].MaxSystemTempC != 71 {
		t.Fatalf("second notified max temp = %v, want 71", states[1].MaxSystemTempC)
	}
}

func TestMonitor_CPUSpikeHold(t *testing.T) {
	start := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	sampler := &fakeSystemSampler{states: []State{
		stateWithMetrics(intPtr(20), intPtr(32)),
		stateWithMetrics(intPtr(85), intPtr(32)),
		stateWithMetrics(intPtr(30), intPtr(32)),
		stateWithMetrics(intPtr(30), intPtr(32)),
	}}
	m := NewMonitorWithSampler(time.Second, sampler)
	m.now = func() time.Time { return start }

	m.poll()
	if m.GetState().CPUSpiking {
		t.Fatal("CPU should not be spiking before threshold crossing")
	}

	start = start.Add(time.Second)
	m.poll()
	if !m.GetState().CPUSpiking {
		t.Fatal("CPU should be spiking after threshold crossing")
	}

	start = start.Add(5 * time.Second)
	m.poll()
	if !m.GetState().CPUSpiking {
		t.Fatal("CPU spike should be held for the hold window")
	}

	start = start.Add(11 * time.Second)
	m.poll()
	if m.GetState().CPUSpiking {
		t.Fatal("CPU spike should expire after the hold window")
	}
}

func TestMonitor_CPUPeakWindow(t *testing.T) {
	start := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	sampler := &fakeSystemSampler{states: []State{
		stateWithMetrics(intPtr(40), intPtr(32)),
		stateWithMetrics(intPtr(70), intPtr(32)),
		stateWithMetrics(intPtr(10), intPtr(32)),
	}}
	m := NewMonitorWithSampler(time.Second, sampler)
	m.cpuPeakWindow = time.Second
	m.now = func() time.Time { return start }

	m.poll()
	if peak := m.GetState().CPUPeakPercent; peak == nil || *peak != 40 {
		t.Fatalf("first peak = %v, want 40", peak)
	}

	start = start.Add(500 * time.Millisecond)
	m.poll()
	if peak := m.GetState().CPUPeakPercent; peak == nil || *peak != 70 {
		t.Fatalf("second peak = %v, want 70", peak)
	}

	start = start.Add(2 * time.Second)
	m.poll()
	if peak := m.GetState().CPUPeakPercent; peak == nil || *peak != 10 {
		t.Fatalf("expired peak = %v, want 10", peak)
	}
}

func TestMonitor_UnavailableClearsSpikeState(t *testing.T) {
	start := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	sampler := &fakeSystemSampler{
		states: []State{
			stateWithMetrics(intPtr(90), intPtr(32)),
			{},
		},
		errs: []error{
			nil,
			fmt.Errorf("proc unavailable"),
		},
	}
	m := NewMonitorWithSampler(time.Second, sampler)
	m.now = func() time.Time { return start }

	m.poll()
	if !m.GetState().CPUSpiking {
		t.Fatal("CPU should be spiking")
	}

	start = start.Add(time.Second)
	m.poll()
	state := m.GetState()
	if state.Available {
		t.Fatal("state should be unavailable")
	}
	if state.CPUSpiking {
		t.Fatal("CPU spike state should clear when unavailable")
	}
}

func TestMonitor_StartStop(t *testing.T) {
	sampler := &fakeSystemSampler{states: []State{stateWithMetrics(nil, intPtr(32))}}
	m := NewMonitorWithSampler(10*time.Millisecond, sampler)
	m.Start()

	time.Sleep(30 * time.Millisecond)
	m.Stop()
	m.Stop()

	if sampler.CallCount() < 2 {
		t.Errorf("expected at least 2 polls, got %d", sampler.CallCount())
	}
}

func stateWithMetrics(cpu, memory *int) State {
	return State{
		Available:        true,
		CPUPercent:       cpu,
		MemoryPercent:    memory,
		MemoryUsedBytes:  320 * 1024,
		MemoryTotalBytes: 1000 * 1024,
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

type fakeSystemSampler struct {
	mu     sync.Mutex
	states []State
	errs   []error
	calls  int
}

func (s *fakeSystemSampler) Sample() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.calls
	s.calls++
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}

	var err error
	if idx < len(s.errs) {
		err = s.errs[idx]
	}
	return s.states[idx], err
}

func (s *fakeSystemSampler) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
