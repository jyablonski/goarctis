package device

import "github.com/jyablonski/goarctis/pkg/protocol"

// DeviceType represents the type of device
type DeviceType string

const (
	DeviceTypeSteelSeriesGameBuds DeviceType = "steelseries_gamebuds"
	DeviceTypeRazerDeathAdder     DeviceType = "razer_deathadder"
)

// BatteryDevice is the interface that all battery-monitoring devices must implement
type BatteryDevice interface {
	// GetID returns a unique identifier for this device instance
	GetID() string

	// GetName returns a human-readable name for the device
	GetName() string

	// GetType returns the device type
	GetType() DeviceType

	// GetState returns the current device state
	GetState() protocol.DeviceState

	// IsConnected returns whether the device is currently connected
	IsConnected() bool

	// Start begins monitoring the device
	Start() error

	// Stop stops monitoring the device
	Stop() error

	// Close releases resources associated with the device
	Close() error

	// SetOnStateChange sets a callback for when device state changes
	SetOnStateChange(callback func(protocol.DeviceState))
}
