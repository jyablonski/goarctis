package device

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

var (
	ErrNoSupportedDevices = errors.New("no supported devices found")
	ErrStartAllDevices    = errors.New("failed to start any devices")
)

type DeviceManager struct {
	devices  map[string]BatteryDevice
	mu       sync.RWMutex
	onChange func(string, protocol.DeviceState)
}

type DiscoveryConfig struct {
	DisableGameBuds bool
	DisableRazer    bool
	DisableHyperX   bool
}

func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		devices: make(map[string]BatteryDevice),
	}
}

func (dm *DeviceManager) SetOnStateChange(callback func(string, protocol.DeviceState)) {
	dm.mu.Lock()
	dm.onChange = callback
	dm.mu.Unlock()
}

func (dm *DeviceManager) DiscoverDevices(cfg DiscoveryConfig) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if cfg.DisableGameBuds {
		log.Println("SteelSeries GameBuds discovery disabled")
	} else {
		steelSeriesDevice := NewHIDRawManager()
		if err := steelSeriesDevice.FindDevices(); err != nil {
			log.Printf("SteelSeries GameBuds not found: %v", err)
		} else {
			steelSeriesDevice.SetOnStateChange(dm.makeStateChangeHandler(steelSeriesDevice.GetID()))
			dm.devices[steelSeriesDevice.GetID()] = steelSeriesDevice
			log.Printf("Found %s", steelSeriesDevice.GetName())
		}
	}

	if cfg.DisableHyperX {
		log.Println("HyperX Cloud Alpha Wireless discovery disabled")
	} else {
		hyperxDevice := NewHyperXDevice()
		if err := hyperxDevice.FindDevice(); err != nil {
			log.Printf("HyperX Cloud Alpha Wireless not found: %v", err)
		} else {
			hyperxDevice.SetOnStateChange(dm.makeStateChangeHandler(hyperxDevice.GetID()))
			dm.devices[hyperxDevice.GetID()] = hyperxDevice
			log.Printf("Found %s", hyperxDevice.GetName())
		}
	}

	if cfg.DisableRazer {
		log.Println("Razer device discovery disabled")
	} else {
		razerDevices, err := DiscoverRazerDevices()
		if err != nil {
			log.Printf("Razer devices not found or OpenRazer not available: %v", err)
		} else {
			for _, razerDevice := range razerDevices {
				razerDevice.SetOnStateChange(dm.makeStateChangeHandler(razerDevice.GetID()))
				dm.devices[razerDevice.GetID()] = razerDevice
				log.Printf("Found %s", razerDevice.GetName())
			}
		}
	}

	if len(dm.devices) == 0 {
		return ErrNoSupportedDevices
	}

	return nil
}

func (dm *DeviceManager) makeStateChangeHandler(deviceID string) func(protocol.DeviceState) {
	return func(state protocol.DeviceState) {
		dm.mu.RLock()
		onChange := dm.onChange
		dm.mu.RUnlock()
		if onChange != nil {
			onChange(deviceID, state)
		}
	}
}

func (dm *DeviceManager) StartAll() error {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var errs []error
	for deviceID, device := range dm.devices {
		if err := device.Start(); err != nil {
			log.Printf("Failed to start device %s: %v", deviceID, err)
			errs = append(errs, fmt.Errorf("device %s: %w", deviceID, err))
		}
	}

	if len(errs) > 0 && len(errs) == len(dm.devices) {
		return fmt.Errorf("%w: %w", ErrStartAllDevices, errors.Join(errs...))
	}

	return nil
}

func (dm *DeviceManager) StopAll() {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, device := range dm.devices {
		if err := device.Stop(); err != nil {
			log.Printf("Error stopping device: %v", err)
		}
	}
}

func (dm *DeviceManager) CloseAll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for _, device := range dm.devices {
		if err := device.Close(); err != nil {
			log.Printf("Error closing device: %v", err)
		}
	}
	dm.devices = make(map[string]BatteryDevice)
}

func (dm *DeviceManager) GetDevice(deviceID string) BatteryDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.devices[deviceID]
}

func (dm *DeviceManager) GetAllDevices() map[string]BatteryDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]BatteryDevice)
	for k, v := range dm.devices {
		result[k] = v
	}
	return result
}

func (dm *DeviceManager) GetDeviceStates() map[string]protocol.DeviceState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	states := make(map[string]protocol.DeviceState)
	for deviceID, device := range dm.devices {
		states[deviceID] = device.GetState()
	}
	return states
}
