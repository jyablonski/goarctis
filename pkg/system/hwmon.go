package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultHwmonRoot = "/sys/class/hwmon"

type HwmonSampler struct {
	root   string
	reader fileReader
}

func NewHwmonSampler() *HwmonSampler {
	return NewHwmonSamplerWithRoot(osFileReader{}, defaultHwmonRoot)
}

func NewHwmonSamplerWithRoot(reader fileReader, root string) *HwmonSampler {
	return &HwmonSampler{reader: reader, root: root}
}

func (s *HwmonSampler) Sample() ([]SensorReading, []GPUStats, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read hwmon root: %w", err)
	}

	var sensors []SensorReading
	gpuTemps := make(map[string]float64)
	gpuNames := make(map[string]string)

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "hwmon") {
			continue
		}

		dir := filepath.Join(s.root, entry.Name())
		chip := strings.TrimSpace(readOptionalFile(s.reader, filepath.Join(dir, "name")))
		if chip == "" {
			chip = entry.Name()
		}

		chipSensors := s.readChipTemps(dir, entry.Name(), chip)
		sensors = append(sensors, chipSensors...)
		if isGPUChip(chip) {
			for _, sensor := range chipSensors {
				if sensor.Hidden {
					continue
				}
				key := filepath.Dir(sensor.Source)
				if current, ok := gpuTemps[key]; !ok || sensor.TemperatureC > current {
					gpuTemps[key] = sensor.TemperatureC
					gpuNames[key] = chip
				}
			}
		}
	}

	sort.Slice(sensors, func(i, j int) bool {
		return sensors[i].ID < sensors[j].ID
	})

	gpus := make([]GPUStats, 0, len(gpuTemps))
	for id, temp := range gpuTemps {
		gpus = append(gpus, GPUStats{
			ID:           id,
			Name:         gpuNames[id],
			TemperatureC: &temp,
		})
	}
	sort.Slice(gpus, func(i, j int) bool {
		return gpus[i].ID < gpus[j].ID
	})
	for i := range gpus {
		gpus[i].Index = i
	}

	return sensors, gpus, nil
}

func (s *HwmonSampler) readChipTemps(dir, hwmonName, chip string) []SensorReading {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sensors []SensorReading
	for _, entry := range entries {
		tempIndex, ok := tempInputIndex(entry.Name())
		if !ok {
			continue
		}

		inputPath := filepath.Join(dir, entry.Name())
		rawInput, err := s.reader.ReadFile(inputPath)
		if err != nil {
			continue
		}
		temp, err := parseHwmonTemp(string(rawInput))
		if err != nil {
			continue
		}

		label := strings.TrimSpace(readOptionalFile(s.reader, filepath.Join(dir, fmt.Sprintf("temp%d_label", tempIndex))))
		if label == "" {
			label = fmt.Sprintf("temp%d", tempIndex)
		}

		sensors = append(sensors, SensorReading{
			ID:           fmt.Sprintf("%s.%s.%s", chip, hwmonName, label),
			Label:        fmt.Sprintf("%s %s", chip, label),
			Source:       inputPath,
			Chip:         chip,
			TemperatureC: temp,
		})
	}
	return sensors
}

func tempInputIndex(name string) (int, bool) {
	if !strings.HasPrefix(name, "temp") || !strings.HasSuffix(name, "_input") {
		return 0, false
	}
	indexText := strings.TrimSuffix(strings.TrimPrefix(name, "temp"), "_input")
	index, err := strconv.Atoi(indexText)
	if err != nil || index <= 0 {
		return 0, false
	}
	return index, true
}

func parseHwmonTemp(data string) (float64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(value) / 1000, nil
}

func readOptionalFile(reader fileReader, path string) string {
	data, err := reader.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// maxTemp folds maxFloat over the visible sensors accepted by include. A nil
// include predicate considers every visible sensor.
func maxTemp(sensors []SensorReading, include func(SensorReading) bool) *float64 {
	var max *float64
	for _, sensor := range sensors {
		if sensor.Hidden || (include != nil && !include(sensor)) {
			continue
		}
		max = maxFloat(max, sensor.TemperatureC)
	}
	return max
}

func maxCPUTemp(sensors []SensorReading) *float64 {
	return maxTemp(sensors, isCPUSensor)
}

func maxGPUTemp(sensors []SensorReading, gpus []GPUStats) *float64 {
	var max *float64
	for _, gpu := range gpus {
		if gpu.TemperatureC != nil {
			max = maxFloat(max, *gpu.TemperatureC)
		}
	}
	if max != nil {
		return max
	}
	return maxTemp(sensors, func(sensor SensorReading) bool {
		return isGPUChip(sensor.Chip)
	})
}

func maxSystemTemp(sensors []SensorReading) *float64 {
	return maxTemp(sensors, nil)
}

func maxFloat(max *float64, value float64) *float64 {
	if max == nil || value > *max {
		return &value
	}
	return max
}

func isCPUSensor(sensor SensorReading) bool {
	chip := strings.ToLower(sensor.Chip)
	label := strings.ToLower(sensor.Label)
	return strings.Contains(chip, "coretemp") ||
		strings.Contains(chip, "k10temp") ||
		strings.Contains(chip, "zenpower") ||
		strings.Contains(chip, "cpu") ||
		strings.Contains(label, "cpu") ||
		strings.Contains(label, "package id")
}

func isGPUChip(chip string) bool {
	chip = strings.ToLower(chip)
	return strings.Contains(chip, "amdgpu") ||
		isNVIDIAChip(chip) ||
		strings.Contains(chip, "nouveau") ||
		strings.Contains(chip, "radeon") ||
		strings.Contains(chip, "i915") ||
		strings.Contains(chip, "xe")
}

func isNVIDIAChip(chip string) bool {
	return strings.Contains(strings.ToLower(chip), "nvidia")
}
