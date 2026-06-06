package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHwmonSampler_ReadsTemperaturesAndGPUStats(t *testing.T) {
	root := t.TempDir()
	writeHwmonFile(t, root, "hwmon0/name", "coretemp\n")
	writeHwmonFile(t, root, "hwmon0/temp1_label", "Package id 0\n")
	writeHwmonFile(t, root, "hwmon0/temp1_input", "64250\n")
	writeHwmonFile(t, root, "hwmon1/name", "nvidia\n")
	writeHwmonFile(t, root, "hwmon1/temp1_label", "GPU\n")
	writeHwmonFile(t, root, "hwmon1/temp1_input", "73000\n")
	writeHwmonFile(t, root, "hwmon1/temp2_label", "Memory\n")
	writeHwmonFile(t, root, "hwmon1/temp2_input", "81000\n")

	sampler := NewHwmonSamplerWithRoot(osFileReader{}, root)
	sensors, gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}

	if len(sensors) != 3 {
		t.Fatalf("sensor count = %d, want 3", len(sensors))
	}
	if len(gpus) != 1 {
		t.Fatalf("gpu count = %d, want 1", len(gpus))
	}
	if gpus[0].TemperatureC == nil || *gpus[0].TemperatureC != 81 {
		t.Fatalf("gpu temp = %v, want 81", gpus[0].TemperatureC)
	}
	if max := maxCPUTemp(sensors); max == nil || *max != 64.25 {
		t.Fatalf("max CPU temp = %v, want 64.25", max)
	}
	if max := maxSystemTemp(sensors); max == nil || *max != 81 {
		t.Fatalf("max system temp = %v, want 81", max)
	}
}

func TestHwmonSampler_MissingRootIsNonFatal(t *testing.T) {
	sampler := NewHwmonSamplerWithRoot(osFileReader{}, filepath.Join(t.TempDir(), "missing"))
	sensors, gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(sensors) != 0 || len(gpus) != 0 {
		t.Fatalf("Sample returned sensors=%d gpus=%d, want none", len(sensors), len(gpus))
	}
}

func TestHwmonSampler_FollowsSysfsClassSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "devices", "platform", "hwmon0")
	writeHwmonFile(t, target, "name", "amdgpu\n")
	writeHwmonFile(t, target, "temp1_input", "67000\n")
	if err := os.Symlink(target, filepath.Join(root, "hwmon0")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	sampler := NewHwmonSamplerWithRoot(osFileReader{}, root)
	sensors, gpus, err := sampler.Sample()
	if err != nil {
		t.Fatalf("Sample returned error: %v", err)
	}
	if len(sensors) != 1 {
		t.Fatalf("sensor count = %d, want 1", len(sensors))
	}
	if len(gpus) != 1 {
		t.Fatalf("gpu count = %d, want 1", len(gpus))
	}
}

func TestParseHwmonTemp(t *testing.T) {
	temp, err := parseHwmonTemp("42500\n")
	if err != nil {
		t.Fatalf("parseHwmonTemp returned error: %v", err)
	}
	if temp != 42.5 {
		t.Fatalf("temp = %v, want 42.5", temp)
	}
}

func writeHwmonFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
