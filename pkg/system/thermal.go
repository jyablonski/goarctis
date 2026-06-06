package system

type DefaultThermalSampler struct {
	hwmon  ThermalSampler
	nvidia *NVIDIASampler
}

func NewDefaultThermalSampler() *DefaultThermalSampler {
	return NewDefaultThermalSamplerWithConfig(GovernorConfig{})
}

// NewDefaultThermalSamplerWithConfig wires the GPU thermal guard config through
// to the NVIDIA sampler. A zero-value (disabled) config behaves like
// NewDefaultThermalSampler.
func NewDefaultThermalSamplerWithConfig(cfg GovernorConfig) *DefaultThermalSampler {
	return &DefaultThermalSampler{
		hwmon:  NewHwmonSampler(),
		nvidia: NewNVIDIASamplerWithConfig(cfg),
	}
}

func NewDefaultThermalSamplerWithProviders(hwmon ThermalSampler, nvidia *NVIDIASampler) *DefaultThermalSampler {
	return &DefaultThermalSampler{hwmon: hwmon, nvidia: nvidia}
}

func (s *DefaultThermalSampler) Sample() ([]SensorReading, []GPUStats, error) {
	var sensors []SensorReading
	var gpus []GPUStats

	if s.hwmon != nil {
		hwmonSensors, hwmonGPUs, err := s.hwmon.Sample()
		if err != nil {
			return nil, nil, err
		}
		sensors = hwmonSensors
		gpus = hwmonGPUs
	}

	if s.nvidia == nil {
		return sensors, gpus, nil
	}

	nvidiaGPUs, err := s.nvidia.Sample()
	if err != nil || len(nvidiaGPUs) == 0 {
		return sensors, gpus, nil
	}

	merged := make([]GPUStats, 0, len(gpus)+len(nvidiaGPUs))
	for _, gpu := range gpus {
		if isNVIDIAChip(gpu.Name) {
			continue
		}
		merged = append(merged, gpu)
	}
	merged = append(merged, nvidiaGPUs...)
	for i := range merged {
		merged[i].Index = i
	}
	return sensors, merged, nil
}

// Close releases the NVIDIA sampler, restoring any thermal-guard clamps.
func (s *DefaultThermalSampler) Close() error {
	if s.nvidia != nil {
		return s.nvidia.Close()
	}
	return nil
}
