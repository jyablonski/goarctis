package docker

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ContainerInfo struct {
	ID   string
	Name string
}

type DockerState struct {
	Available  bool
	Containers []ContainerInfo
}

func (s DockerState) RunningCount() int {
	return len(s.Containers)
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type ExecCommandRunner struct{}

func (r *ExecCommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

type Monitor struct {
	mu       sync.RWMutex
	state    DockerState
	onChange func(DockerState)
	runner   CommandRunner

	pollInterval time.Duration
	stopCh       chan struct{}
	stopped      bool
}

func NewMonitor(interval time.Duration) *Monitor {
	return &Monitor{
		runner:       &ExecCommandRunner{},
		pollInterval: interval,
		stopCh:       make(chan struct{}),
	}
}

func NewMonitorWithRunner(interval time.Duration, runner CommandRunner) *Monitor {
	return &Monitor{
		runner:       runner,
		pollInterval: interval,
		stopCh:       make(chan struct{}),
	}
}

func (m *Monitor) SetOnChange(fn func(DockerState)) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Monitor) GetState() DockerState {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	newState := m.fetchState(ctx)

	m.mu.Lock()
	changed := m.stateChanged(newState)
	m.state = newState
	onChange := m.onChange
	m.mu.Unlock()

	if changed && onChange != nil {
		onChange(newState)
	}
}

func (m *Monitor) fetchState(ctx context.Context) DockerState {
	// Use docker ps with a format template to get structured output
	out, err := m.runner.Run(ctx, "docker", "ps", "--format", "{{.ID}}\t{{.Names}}")
	if err != nil {
		log.Printf("Docker not available: %v", err)
		return DockerState{Available: false}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var containers []ContainerInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		id := parts[0]
		name := id
		if len(parts) == 2 {
			name = parts[1]
		}
		containers = append(containers, ContainerInfo{ID: id, Name: name})
	}

	return DockerState{
		Available:  true,
		Containers: containers,
	}
}

// stateChanged returns true if the new state differs from the current state.
// Must be called with m.mu held.
func (m *Monitor) stateChanged(newState DockerState) bool {
	if m.state.Available != newState.Available {
		return true
	}
	if len(m.state.Containers) != len(newState.Containers) {
		return true
	}
	oldIDs := make(map[string]bool, len(m.state.Containers))
	for _, c := range m.state.Containers {
		oldIDs[c.ID] = true
	}
	for _, c := range newState.Containers {
		if !oldIDs[c.ID] {
			return true
		}
	}
	return false
}

func StopAllContainers(runner CommandRunner) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := runner.Run(ctx, "docker", "ps", "-q")
	if err != nil {
		return 0, err
	}

	ids := strings.Fields(strings.TrimSpace(out))
	if len(ids) == 0 {
		return 0, nil
	}

	args := append([]string{"stop"}, ids...)
	_, err = runner.Run(ctx, "docker", args...)
	if err != nil {
		return 0, err
	}

	return len(ids), nil
}
