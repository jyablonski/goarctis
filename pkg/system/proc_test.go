package system

import (
	"fmt"
	"testing"
)

func TestParseMemInfo_WithMemAvailable(t *testing.T) {
	data := `MemTotal:       1000 kB
MemFree:         100 kB
MemAvailable:    680 kB
Buffers:          50 kB
Cached:          200 kB
`

	used, total, percent, err := parseMemInfo(data)
	if err != nil {
		t.Fatalf("parseMemInfo returned error: %v", err)
	}
	if total != 1000*1024 {
		t.Errorf("total = %d, want %d", total, 1000*1024)
	}
	if used != 320*1024 {
		t.Errorf("used = %d, want %d", used, 320*1024)
	}
	if percent != 32 {
		t.Errorf("percent = %d, want 32", percent)
	}
}

func TestParseMemInfo_FallbackWithoutMemAvailable(t *testing.T) {
	data := `MemTotal:       1000 kB
MemFree:         100 kB
Buffers:          50 kB
Cached:          200 kB
SReclaimable:     30 kB
Shmem:            10 kB
`

	used, _, percent, err := parseMemInfo(data)
	if err != nil {
		t.Fatalf("parseMemInfo returned error: %v", err)
	}
	if used != 630*1024 {
		t.Errorf("used = %d, want %d", used, 630*1024)
	}
	if percent != 63 {
		t.Errorf("percent = %d, want 63", percent)
	}
}

func TestParseMemInfo_MissingTotal(t *testing.T) {
	_, _, _, err := parseMemInfo("MemAvailable: 100 kB\n")
	if err == nil {
		t.Fatal("expected error for missing MemTotal")
	}
}

func TestParseCPUTimes(t *testing.T) {
	times, err := parseCPUTimes("cpu  100 10 40 850 20 5 5 0\ncpu0 1 2 3 4\n")
	if err != nil {
		t.Fatalf("parseCPUTimes returned error: %v", err)
	}
	if times.idle != 870 {
		t.Errorf("idle = %d, want 870", times.idle)
	}
	if times.total != 1030 {
		t.Errorf("total = %d, want 1030", times.total)
	}
}

func TestCalculateCPUPercent(t *testing.T) {
	prev := cpuTimes{idle: 800, total: 1000}
	current := cpuTimes{idle: 900, total: 1200}

	percent, ok := calculateCPUPercent(prev, current)
	if !ok {
		t.Fatal("calculateCPUPercent returned !ok")
	}
	if percent != 50 {
		t.Errorf("percent = %d, want 50", percent)
	}
}

func TestCalculateCPUPercent_InvalidDelta(t *testing.T) {
	_, ok := calculateCPUPercent(cpuTimes{idle: 100, total: 200}, cpuTimes{idle: 90, total: 210})
	if ok {
		t.Fatal("calculateCPUPercent should reject decreasing idle counters")
	}
}

func TestProcSampler_FirstAndSecondSample(t *testing.T) {
	reader := &fakeProcReader{
		meminfo: `MemTotal: 1000 kB
MemAvailable: 680 kB
`,
		stats: []string{
			"cpu  100 0 100 800 0 0 0 0\n",
			"cpu  150 0 150 900 0 0 0 0\n",
		},
	}
	sampler := NewProcSamplerWithReader(reader)

	first, err := sampler.Sample()
	if err != nil {
		t.Fatalf("first sample returned error: %v", err)
	}
	if !first.Available {
		t.Fatal("first sample should be available")
	}
	if first.CPUPercent != nil {
		t.Errorf("first CPUPercent = %v, want nil", *first.CPUPercent)
	}
	if first.MemoryPercent == nil || *first.MemoryPercent != 32 {
		t.Fatalf("first MemoryPercent = %v, want 32", first.MemoryPercent)
	}

	second, err := sampler.Sample()
	if err != nil {
		t.Fatalf("second sample returned error: %v", err)
	}
	if second.CPUPercent == nil || *second.CPUPercent != 50 {
		t.Fatalf("second CPUPercent = %v, want 50", second.CPUPercent)
	}
}

type fakeProcReader struct {
	meminfo   string
	stats     []string
	statCalls int
}

func (r *fakeProcReader) ReadFile(name string) ([]byte, error) {
	switch name {
	case "/proc/meminfo":
		return []byte(r.meminfo), nil
	case "/proc/stat":
		if r.statCalls >= len(r.stats) {
			return nil, fmt.Errorf("no stat sample %d", r.statCalls)
		}
		value := r.stats[r.statCalls]
		r.statCalls++
		return []byte(value), nil
	default:
		return nil, fmt.Errorf("unexpected file: %s", name)
	}
}
