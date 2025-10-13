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
	devices  []io.ReadCloser
	protocol *protocol.Handler
	stopChan chan struct{}
	fs       FileSystem
}

func NewHIDRawManager() *HIDRawManager {
	return &HIDRawManager{
		protocol: protocol.NewHandler(),
		stopChan: make(chan struct{}),
		fs:       RealFileSystem{},
	}
}

// NewHIDRawManagerWithFS creates a manager with a custom filesystem (for testing)
func NewHIDRawManagerWithFS(fs FileSystem) *HIDRawManager {
	return &HIDRawManager{
		protocol: protocol.NewHandler(),
		stopChan: make(chan struct{}),
		fs:       fs,
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

// SetOnStateChange sets a callback for when device state changes
func (m *HIDRawManager) SetOnStateChange(callback func(protocol.DeviceState)) {
	m.protocol.SetOnChange(callback)
}

// Listen starts monitoring all HID interfaces
func (m *HIDRawManager) Listen() {
	if len(m.devices) == 0 {
		log.Println("No devices to monitor")
		return
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

	// Wait for stop signal
	<-m.stopChan
}

// GetState returns the current device state
func (m *HIDRawManager) GetState() protocol.DeviceState {
	return m.protocol.GetState()
}

// Stop stops monitoring
func (m *HIDRawManager) Stop() {
	close(m.stopChan)
}

// Close closes all devices
func (m *HIDRawManager) Close() {
	m.Stop()
	for _, dev := range m.devices {
		dev.Close()
	}
}
