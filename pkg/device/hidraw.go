package device

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

const (
	VendorID  = 0x1038
	ProductID = 0x230a
)

// FileSystem interface for testability
type FileSystem interface {
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	OpenFile(name string, flag int, perm os.FileMode) (io.ReadCloser, error)
}

// RealFileSystem implements FileSystem using actual OS calls
type RealFileSystem struct{}

func (fs RealFileSystem) ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

func (fs RealFileSystem) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

func (fs RealFileSystem) OpenFile(name string, flag int, perm os.FileMode) (io.ReadCloser, error) {
	return os.OpenFile(name, flag, perm)
}

type HIDRawManager struct {
	devices    []io.ReadCloser
	protocol   *protocol.Handler
	stopChan   chan struct{}
	fs         FileSystem
	deviceID   string
	deviceName string
	onChange   func(protocol.DeviceState)
}

func NewHIDRawManager() *HIDRawManager {
	return &HIDRawManager{
		protocol:   protocol.NewHandler(),
		stopChan:   make(chan struct{}),
		fs:         RealFileSystem{},
		deviceID:   "steelseries_gamebuds",
		deviceName: "SteelSeries Arctis GameBuds",
	}
}

// NewHIDRawManagerWithFS creates a manager with a custom filesystem (for testing)
func NewHIDRawManagerWithFS(fs FileSystem) *HIDRawManager {
	return &HIDRawManager{
		protocol:   protocol.NewHandler(),
		stopChan:   make(chan struct{}),
		fs:         fs,
		deviceID:   "steelseries_gamebuds",
		deviceName: "SteelSeries Arctis GameBuds",
	}
}

// FindDevices finds all hidraw devices for the GameBuds
func (m *HIDRawManager) FindDevices() error {
	files, err := m.fs.ReadDir("/sys/class/hidraw")
	if err != nil {
		return fmt.Errorf("failed to read hidraw devices: %w", err)
	}

	var hidrawPaths []string
	for _, f := range files {
		devicePath := fmt.Sprintf("/sys/class/hidraw/%s/device/uevent", f.Name())
		data, err := m.fs.ReadFile(devicePath)
		if err != nil {
			continue
		}

		content := string(data)
		// Look for our VID:PID (1038:230a)
		if strings.Contains(content, "HID_ID=0003:00001038:0000230A") {
			hidrawPath := fmt.Sprintf("/dev/%s", f.Name())
			hidrawPaths = append(hidrawPaths, hidrawPath)

			// Get interface number
			interfacePath := fmt.Sprintf("/sys/class/hidraw/%s/device/../bInterfaceNumber", f.Name())
			if ifNum, err := m.fs.ReadFile(interfacePath); err == nil {
				ifNumStr := strings.TrimSpace(string(ifNum))
				log.Printf("Found GameBuds HID interface: %s (interface %s)", hidrawPath, ifNumStr)
			}
		}
	}

	if len(hidrawPaths) == 0 {
		return fmt.Errorf("no GameBuds hidraw devices found")
	}

	// Open all devices
	for _, path := range hidrawPaths {
		f, err := m.fs.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			log.Printf("Warning: Could not open %s: %v", path, err)
			continue
		}
		m.devices = append(m.devices, f)
	}

	if len(m.devices) == 0 {
		return fmt.Errorf("could not open any hidraw devices")
	}

	log.Printf("Successfully opened %d HID interfaces", len(m.devices))
	return nil
}

// GetID returns the device identifier
func (m *HIDRawManager) GetID() string {
	return m.deviceID
}

// GetName returns the device name
func (m *HIDRawManager) GetName() string {
	return m.deviceName
}

// GetType returns the device type
func (m *HIDRawManager) GetType() DeviceType {
	return DeviceTypeSteelSeriesGameBuds
}

// SetOnStateChange sets a callback for when device state changes
func (m *HIDRawManager) SetOnStateChange(callback func(protocol.DeviceState)) {
	m.onChange = callback
	// Wrap callback to set device ID and type
	m.protocol.SetOnChange(func(state protocol.DeviceState) {
		state.DeviceID = m.deviceID
		state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
		if m.onChange != nil {
			m.onChange(state)
		}
	})
}

// Start begins monitoring all HID interfaces
func (m *HIDRawManager) Start() error {
	if len(m.devices) == 0 {
		return fmt.Errorf("no devices to monitor")
	}

	log.Printf("Monitoring %d HID interfaces...", len(m.devices))

	// Start a goroutine for each device
	for i, device := range m.devices {
		deviceNum := i
		dev := device

		go func() {
			buf := make([]byte, 64)
			for {
				select {
				case <-m.stopChan:
					return
				default:
					n, err := dev.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.Printf("Read error on device %d: %v", deviceNum, err)
						}
						return
					}

					if n > 0 {
						data := make([]byte, n)
						copy(data, buf[:n])
						m.protocol.ParseReport(data)
					}
				}
			}
		}()
	}

	return nil
}

// GetState returns the current device state
func (m *HIDRawManager) GetState() protocol.DeviceState {
	state := m.protocol.GetState()
	state.DeviceID = m.deviceID
	state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
	return state
}

// IsConnected returns whether the device is connected
func (m *HIDRawManager) IsConnected() bool {
	return len(m.devices) > 0
}

// Stop stops monitoring
func (m *HIDRawManager) Stop() error {
	select {
	case <-m.stopChan:
		// Already closed
	default:
		close(m.stopChan)
	}
	return nil
}

// Close closes all devices
func (m *HIDRawManager) Close() error {
	m.Stop()
	for _, dev := range m.devices {
		dev.Close()
	}
	return nil
}
