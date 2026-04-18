package docker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type MockCommandRunner struct {
	mu       sync.Mutex
	calls    []mockCall
	handlers map[string]func(args []string) (string, error)
}

type mockCall struct {
	Name string
	Args []string
}

func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		handlers: make(map[string]func(args []string) (string, error)),
	}
}

func (m *MockCommandRunner) OnCommand(name string, handler func(args []string) (string, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[name] = handler
}

func (m *MockCommandRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	handler, ok := m.handlers[name]
	m.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("command not found: %s", name)
	}
	return handler(args)
}

func (m *MockCommandRunner) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *MockCommandRunner) GetCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func TestDockerState_RunningCount(t *testing.T) {
	tests := []struct {
		name     string
		state    DockerState
		expected int
	}{
		{
			name:     "No containers",
			state:    DockerState{Available: true, Containers: nil},
			expected: 0,
		},
		{
			name: "One container",
			state: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "abc123", Name: "web"}},
			},
			expected: 1,
		},
		{
			name: "Multiple containers",
			state: DockerState{
				Available: true,
				Containers: []ContainerInfo{
					{ID: "abc123", Name: "web"},
					{ID: "def456", Name: "db"},
					{ID: "ghi789", Name: "redis"},
				},
			},
			expected: 3,
		},
		{
			name:     "Docker unavailable",
			state:    DockerState{Available: false},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.RunningCount()
			if got != tt.expected {
				t.Errorf("RunningCount() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestNewMonitor(t *testing.T) {
	m := NewMonitor(5 * time.Second)
	if m == nil {
		t.Fatal("NewMonitor returned nil")
	}
	if m.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", m.pollInterval)
	}
	if m.runner == nil {
		t.Error("runner should not be nil")
	}
}

func TestNewMonitorWithRunner(t *testing.T) {
	runner := NewMockCommandRunner()
	m := NewMonitorWithRunner(3*time.Second, runner)
	if m == nil {
		t.Fatal("NewMonitorWithRunner returned nil")
	}
	if m.pollInterval != 3*time.Second {
		t.Errorf("pollInterval = %v, want 3s", m.pollInterval)
	}
}

func TestMonitor_FetchState_NoContainers(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)
	state := m.GetState()

	if state.Available {
		t.Error("Should not be available before start")
	}

	m.poll()
	state = m.GetState()

	if !state.Available {
		t.Error("Should be available after successful docker command")
	}
	if state.RunningCount() != 0 {
		t.Errorf("RunningCount = %d, want 0", state.RunningCount())
	}
}

func TestMonitor_FetchState_WithContainers(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "abc123def456\tweb-app\nghi789012345\tpostgres-db\n", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)
	m.poll()
	state := m.GetState()

	if !state.Available {
		t.Error("Should be available")
	}
	if state.RunningCount() != 2 {
		t.Errorf("RunningCount = %d, want 2", state.RunningCount())
	}
	if state.Containers[0].ID != "abc123def456" {
		t.Errorf("Container[0].ID = %q, want %q", state.Containers[0].ID, "abc123def456")
	}
	if state.Containers[0].Name != "web-app" {
		t.Errorf("Container[0].Name = %q, want %q", state.Containers[0].Name, "web-app")
	}
	if state.Containers[1].Name != "postgres-db" {
		t.Errorf("Container[1].Name = %q, want %q", state.Containers[1].Name, "postgres-db")
	}
}

func TestMonitor_FetchState_DockerUnavailable(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", fmt.Errorf("Cannot connect to the Docker daemon")
	})

	m := NewMonitorWithRunner(time.Second, runner)
	m.poll()
	state := m.GetState()

	if state.Available {
		t.Error("Should not be available when docker command fails")
	}
	if state.RunningCount() != 0 {
		t.Errorf("RunningCount = %d, want 0", state.RunningCount())
	}
}

func TestMonitor_OnChange_CalledOnStateChange(t *testing.T) {
	runner := NewMockCommandRunner()
	callCount := 0
	runner.OnCommand("docker", func(args []string) (string, error) {
		callCount++
		if callCount == 1 {
			return "abc123\tweb\n", nil
		}
		return "abc123\tweb\ndef456\tdb\n", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)

	var receivedStates []DockerState
	var mu sync.Mutex
	m.SetOnChange(func(state DockerState) {
		mu.Lock()
		receivedStates = append(receivedStates, state)
		mu.Unlock()
	})

	m.poll()
	mu.Lock()
	if len(receivedStates) != 1 {
		t.Fatalf("Expected 1 callback after first poll, got %d", len(receivedStates))
	}
	if receivedStates[0].RunningCount() != 1 {
		t.Errorf("First state RunningCount = %d, want 1", receivedStates[0].RunningCount())
	}
	mu.Unlock()

	m.poll()
	mu.Lock()
	if len(receivedStates) != 2 {
		t.Fatalf("Expected 2 callbacks after second poll, got %d", len(receivedStates))
	}
	if receivedStates[1].RunningCount() != 2 {
		t.Errorf("Second state RunningCount = %d, want 2", receivedStates[1].RunningCount())
	}
	mu.Unlock()
}

func TestMonitor_OnChange_NotCalledWhenUnchanged(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "abc123\tweb\n", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)

	changeCount := 0
	var mu sync.Mutex
	m.SetOnChange(func(state DockerState) {
		mu.Lock()
		changeCount++
		mu.Unlock()
	})

	m.poll()
	m.poll()

	mu.Lock()
	if changeCount != 1 {
		t.Errorf("Expected 1 change callback for duplicate state, got %d", changeCount)
	}
	mu.Unlock()
}

func TestMonitor_StartStop(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", nil
	})

	m := NewMonitorWithRunner(50*time.Millisecond, runner)
	m.Start()

	time.Sleep(200 * time.Millisecond)
	m.Stop()

	if runner.CallCount() < 2 {
		t.Errorf("Expected at least 2 polls, got %d", runner.CallCount())
	}
}

func TestMonitor_StopIdempotent(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)
	m.Start()
	m.Stop()
	m.Stop()
}

func TestMonitor_StateChanged(t *testing.T) {
	m := &Monitor{}

	tests := []struct {
		name     string
		old      DockerState
		new      DockerState
		expected bool
	}{
		{
			name:     "Availability changed",
			old:      DockerState{Available: false},
			new:      DockerState{Available: true},
			expected: true,
		},
		{
			name: "Container count changed",
			old: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "a", Name: "web"}},
			},
			new: DockerState{
				Available: true,
				Containers: []ContainerInfo{
					{ID: "a", Name: "web"},
					{ID: "b", Name: "db"},
				},
			},
			expected: true,
		},
		{
			name: "Container IDs changed",
			old: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "a", Name: "web"}},
			},
			new: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "b", Name: "api"}},
			},
			expected: true,
		},
		{
			name: "No change",
			old: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "a", Name: "web"}},
			},
			new: DockerState{
				Available:  true,
				Containers: []ContainerInfo{{ID: "a", Name: "web"}},
			},
			expected: false,
		},
		{
			name:     "Both empty",
			old:      DockerState{Available: true},
			new:      DockerState{Available: true},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.state = tt.old
			got := m.stateChanged(tt.new)
			if got != tt.expected {
				t.Errorf("stateChanged() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStopAllContainers_NoContainers(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", nil
	})

	stopped, err := StopAllContainers(runner)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if stopped != 0 {
		t.Errorf("Stopped = %d, want 0", stopped)
	}
}

func TestStopAllContainers_WithContainers(t *testing.T) {
	runner := NewMockCommandRunner()
	callNum := 0
	runner.OnCommand("docker", func(args []string) (string, error) {
		callNum++
		if callNum == 1 {
			if len(args) >= 2 && args[0] == "ps" && args[1] == "-q" {
				return "abc123\ndef456\n", nil
			}
		}
		if callNum == 2 {
			if len(args) >= 3 && args[0] == "stop" {
				return "abc123\ndef456\n", nil
			}
		}
		return "", fmt.Errorf("unexpected call: %v", args)
	})

	stopped, err := StopAllContainers(runner)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if stopped != 2 {
		t.Errorf("Stopped = %d, want 2", stopped)
	}
}

func TestStopAllContainers_DockerError(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "", fmt.Errorf("Docker daemon not running")
	})

	_, err := StopAllContainers(runner)
	if err == nil {
		t.Error("Expected error when Docker is unavailable")
	}
}

func TestMonitor_FetchState_ContainerWithoutTab(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.OnCommand("docker", func(args []string) (string, error) {
		return "abc123def456\n", nil
	})

	m := NewMonitorWithRunner(time.Second, runner)
	m.poll()
	state := m.GetState()

	if state.RunningCount() != 1 {
		t.Errorf("RunningCount = %d, want 1", state.RunningCount())
	}
	if state.Containers[0].ID != "abc123def456" {
		t.Errorf("Container ID = %q, want %q", state.Containers[0].ID, "abc123def456")
	}
	if state.Containers[0].Name != "abc123def456" {
		t.Errorf("Container Name = %q, want %q (should fall back to ID)", state.Containers[0].Name, "abc123def456")
	}
}
