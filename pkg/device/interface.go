package device

import "github.com/jyablonski/goarctis/pkg/protocol"

type DeviceType string

const (
	DeviceTypeSteelSeriesGameBuds DeviceType = protocol.DeviceTypeSteelSeriesGameBuds
	DeviceTypeRazer               DeviceType = protocol.DeviceTypeRazer
	DeviceTypeHyperXCloudAlpha    DeviceType = protocol.DeviceTypeHyperXCloudAlpha
)

type BatteryDevice interface {
	GetID() string

	GetName() string

	GetType() DeviceType

	GetState() protocol.DeviceState

	IsConnected() bool

	Start() error

	Stop() error

	Close() error

	SetOnStateChange(callback func(protocol.DeviceState))
}
