package device

import (
	"fmt"
	"log"
	"sync"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

// DeviceManager manages multiple battery devices
type DeviceManager struct {
	devices  map[string]BatteryDevice
	mu       sync.RWMutex
	onChange func(string, protocol.DeviceState)
}

// NewDeviceManager creates a new device manager
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		devices: make(map[string]BatteryDevice),
	}
}

// SetOnStateChange sets a callback for when any device state changes
// The callback receives (deviceID, state)
func (dm *DeviceManager) SetOnStateChange(callback func(string, protocol.DeviceState)) {
	dm.mu.Lock()
	dm.onChange = callback
	dm.mu.Unlock()
}

// DiscoverDevices discovers all supported devices
func (dm *DeviceManager) DiscoverDevices() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Discover SteelSeries GameBuds
	steelSeriesDevice := NewHIDRawManager()
	if err := steelSeriesDevice.FindDevices(); err != nil {
		log.Printf("SteelSeries GameBuds not found: %v", err)
	} else {
		steelSeriesDevice.SetOnStateChange(dm.makeStateChangeHandler(steelSeriesDevice.GetID()))
		dm.devices[steelSeriesDevice.GetID()] = steelSeriesDevice
		log.Printf("Found %s", steelSeriesDevice.GetName())
	}

	// Discover Razer devices
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

	if len(dm.devices) == 0 {
		return fmt.Errorf("no supported devices found")
	}

	return nil
}

// makeStateChangeHandler creates a state change handler for a specific device
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

// StartAll starts monitoring all discovered devices
func (dm *DeviceManager) StartAll() error {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var errors []error
	for deviceID, device := range dm.devices {
		if err := device.Start(); err != nil {
			log.Printf("Failed to start device %s: %v", deviceID, err)
			errors = append(errors, fmt.Errorf("device %s: %w", deviceID, err))
		}
	}

	if len(errors) > 0 && len(errors) == len(dm.devices) {
		return fmt.Errorf("failed to start any devices: %v", errors)
	}

	return nil
}

// StopAll stops monitoring all devices
func (dm *DeviceManager) StopAll() {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	for _, device := range dm.devices {
		device.Stop()
	}
}

// CloseAll closes all devices and releases resources
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

// GetDevice returns a device by ID
func (dm *DeviceManager) GetDevice(deviceID string) BatteryDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.devices[deviceID]
}

// GetAllDevices returns all managed devices
func (dm *DeviceManager) GetAllDevices() map[string]BatteryDevice {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]BatteryDevice)
	for k, v := range dm.devices {
		result[k] = v
	}
	return result
}

// GetDeviceStates returns the current state of all devices
func (dm *DeviceManager) GetDeviceStates() map[string]protocol.DeviceState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	states := make(map[string]protocol.DeviceState)
	for deviceID, device := range dm.devices {
		states[deviceID] = device.GetState()
	}
	return states
}
