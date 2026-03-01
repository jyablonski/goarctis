package device

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

const (
	VendorID  = 0x1038
	ProductID = 0x230a

	// HID ioctl constants (from linux/hidraw.h)
	_HIDIOCGRDESCSIZE = 0x01
	_HIDIOCGRDESC     = 0x02
	_HIDIOCGRAWINFO   = 0x03
	_HIDIOCSFEATURE   = 0x06
	_HIDIOCGFEATURE   = 0x07

	// Polling interval for feature report requests
	gameBudsPollInterval = 5 * time.Second
)

// HID ioctl helper functions
func _IOC(dir, typ, nr, size uint) uint {
	return (dir << 30) | (typ << 8) | nr | (size << 16)
}

func _IOR(typ, nr, size uint) uint {
	return _IOC(2, typ, nr, size) // _IOC_READ
}

func _IOW(typ, nr, size uint) uint {
	return _IOC(1, typ, nr, size) // _IOC_WRITE
}

func _IOWR(typ, nr, size uint) uint {
	return _IOC(3, typ, nr, size) // _IOC_READ|_IOC_WRITE
}

func HIDIOCGFEATURE(length uint) uint {
	return _IOWR('H', _HIDIOCGFEATURE, length)
}

func HIDIOCSFEATURE(length uint) uint {
	return _IOWR('H', _HIDIOCSFEATURE, length)
}

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

type deviceInfo struct {
	file   io.ReadCloser
	path   string
	number int
	fd     *os.File // For ioctl operations
}

type HIDRawManager struct {
	devices    []deviceInfo
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
	var interfaceNumbers []string
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

			// Get interface number
			interfacePath := fmt.Sprintf("/sys/class/hidraw/%s/device/../bInterfaceNumber", f.Name())
			ifNumStr := "unknown"
			if ifNum, err := m.fs.ReadFile(interfacePath); err == nil {
				ifNumStr = strings.TrimSpace(string(ifNum))
			}

			// Store with interface number for sorting/prioritization
			hidrawPaths = append(hidrawPaths, hidrawPath)
			interfaceNumbers = append(interfaceNumbers, ifNumStr)
			log.Printf("Found GameBuds HID interface: %s (interface %s)", hidrawPath, ifNumStr)
		}
	}

	// Sort by interface number - interface 03 typically receives status reports
	// Keep original order but log which one we expect to receive data
	for i, ifNum := range interfaceNumbers {
		if ifNum == "03" {
			log.Printf("Interface %s (%s) is typically the status report interface", ifNum, hidrawPaths[i])
		}
	}

	if len(hidrawPaths) == 0 {
		return fmt.Errorf("no GameBuds hidraw devices found")
	}

	// Open all devices
	// Note: On some systems (like Arch Linux), devices may need to be opened in read-write mode
	// to receive certain HID reports. Try read-write first, fall back to read-only if that fails.
	for i, path := range hidrawPaths {
		var f io.ReadCloser
		var err error

		// Try read-write mode first (required for ioctl operations)
		f, err = m.fs.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			// Fall back to read-only
			log.Printf("Could not open %s in read-write mode, trying read-only: %v", path, err)
			f, err = m.fs.OpenFile(path, os.O_RDONLY, 0)
			if err != nil {
				log.Printf("Warning: Could not open %s: %v", path, err)
				continue
			}
			log.Printf("Opened %s in read-only mode (device index %d) - feature reports disabled", path, i)
		} else {
			log.Printf("Opened %s in read-write mode (device index %d) - feature reports enabled", path, i)
		}

		// Get *os.File for ioctl operations if it's a real file
		var fd *os.File
		if realFile, ok := f.(*os.File); ok {
			fd = realFile
		}

		m.devices = append(m.devices, deviceInfo{
			file:   f,
			path:   path,
			number: i,
			fd:     fd,
		})
		log.Printf("Device %d ready - reports may only come on events like earbud removal/placement or ANC mode changes", i)
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

	// Try to request initial feature reports to "wake up" the device
	// This might be required on some systems (like Arch Linux) to start receiving reports
	m.requestInitialFeatureReports()

	// Send initial state update so tray knows device exists
	// This ensures the device appears in the tray even before any reports are received
	// We trigger the protocol handler's callback mechanism to ensure proper state propagation
	initialState := m.GetState()
	log.Printf("🎧 GameBuds: Sending initial state update (DeviceID: %s, DeviceType: %s, IsConnected: %v)",
		initialState.DeviceID, initialState.DeviceType, initialState.IsConnected)
	// Trigger the protocol handler's onChange callback by calling it directly
	// The protocol handler's callback is already set up to call m.onChange with proper DeviceID/DeviceType
	if m.protocol != nil {
		// Manually trigger the callback with current state
		// The protocol handler's onChange is wrapped to set DeviceID/DeviceType
		state := m.protocol.GetState()
		state.DeviceID = m.deviceID
		state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
		if m.onChange != nil {
			m.onChange(state)
		}
	}

	// Start polling goroutine for feature report requests (if we have a writable device)
	m.startPollingGoroutine()

	// Start a goroutine for each device to read incoming reports
	for _, deviceInfo := range m.devices {
		info := deviceInfo // Capture for goroutine

		go func() {
			buf := make([]byte, 64)
			log.Printf("🎧 GameBuds: Started reading goroutine for device %d (%s) - waiting for HID reports...", info.number, info.path)
			log.Printf("🎧 GameBuds: Tip: Try removing/placing earbuds or changing ANC mode to trigger reports")
			log.Printf("🎧 GameBuds: Device %d is in blocking read mode - will wait for data from device", info.number)

			lastLogTime := time.Now()
			readCount := 0
			firstRead := true

			for {
				select {
				case <-m.stopChan:
					log.Printf("🎧 GameBuds: Stopping read goroutine for device %d (%s) - processed %d reports", info.number, info.path, readCount)
					return
				default:
					// Read is blocking - will wait until device sends data
					// GameBuds typically only sends reports when events occur (ANC mode change, earbud removal, etc.)
					if firstRead {
						log.Printf("🎧 GameBuds: Device %d entering blocking read (this is normal - waiting for device to send data)...", info.number)
						firstRead = false
					}
					n, err := info.file.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.Printf("🎧 GameBuds: Read error on device %d (%s): %v", info.number, info.path, err)
						}
						// On error, wait a bit before retrying
						time.Sleep(100 * time.Millisecond)
						continue
					}

					if n > 0 {
						readCount++
						data := make([]byte, n)
						copy(data, buf[:n])
						log.Printf("🎧 GameBuds: ✅ Received HID report #%d from device %d (%s): reportID=0x%02X, data=%x", readCount, info.number, info.path, data[0], data)
						m.protocol.ParseReport(data)
						lastLogTime = time.Now()
					} else {
						// Periodic log to show we're still running
						if time.Since(lastLogTime) > 30*time.Second {
							log.Printf("🎧 GameBuds: Device %d (%s) still waiting for reports (try changing ANC mode or removing earbuds)", info.number, info.path)
							lastLogTime = time.Now()
						}
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

// requestFeatureReport requests a HID feature report from the device
// reportID: The report ID to request (0xB7 for battery, 0xB5 for wear status, 0xBD for ANC mode)
func (m *HIDRawManager) requestFeatureReport(fd *os.File, reportID byte) ([]byte, error) {
	if fd == nil {
		return nil, fmt.Errorf("file descriptor not available")
	}

	buf := make([]byte, 64)
	buf[0] = reportID // First byte is the report ID

	// Use ioctl to request feature report
	ioctl := HIDIOCGFEATURE(uint(len(buf)))
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd.Fd()),
		uintptr(ioctl),
		uintptr(unsafe.Pointer(&buf[0])),
	)

	if errno != 0 {
		return nil, fmt.Errorf("ioctl failed: %v", errno)
	}

	return buf, nil
}

// requestInitialFeatureReports tries to request feature reports on startup
// This may be required to "wake up" the device on some systems
func (m *HIDRawManager) requestInitialFeatureReports() {
	// Try to request feature reports from the first writable device
	for _, devInfo := range m.devices {
		if devInfo.fd != nil {
			log.Printf("🎧 GameBuds: Attempting to request initial feature reports from device %d", devInfo.number)

			// Try common report IDs that might trigger status updates
			// Note: These might be INPUT reports, not FEATURE reports
			// Feature reports return all zeros, suggesting the device uses input reports instead
			reportIDs := []byte{
				0xB7, // Battery report
				0xB5, // Wear status report
				0xBD, // ANC mode report
			}

			for _, reportID := range reportIDs {
				data, err := m.requestFeatureReport(devInfo.fd, reportID)
				if err != nil {
					log.Printf("🎧 GameBuds: Feature report request for 0x%02X failed: %v", reportID, err)
				} else {
					// Check if we got meaningful data (not all zeros)
					allZeros := true
					for _, b := range data {
						if b != 0 {
							allZeros = false
							break
						}
					}
					if allZeros {
						log.Printf("🎧 GameBuds: Feature report 0x%02X returned all zeros - device may not support feature reports for this ID", reportID)
					} else {
						log.Printf("🎧 GameBuds: Received feature report 0x%02X: %x", reportID, data)
						// Parse the received data as if it came from a normal report
						if len(data) > 0 && data[0] == reportID {
							m.protocol.ParseReport(data)
						}
					}
				}
			}

			log.Printf("🎧 GameBuds: Note: Device appears to use INPUT reports (unsolicited), not FEATURE reports")
			log.Printf("🎧 GameBuds: Polling will continue but may not return data - waiting for device to send input reports")
			break // Only try from first writable device
		}
	}
}

// startPollingGoroutine starts a goroutine that periodically requests feature reports
// This implements active polling similar to how Razer devices work
func (m *HIDRawManager) startPollingGoroutine() {
	// Find a writable device for polling
	var pollDevice *deviceInfo
	for i := range m.devices {
		if m.devices[i].fd != nil {
			pollDevice = &m.devices[i]
			break
		}
	}

	if pollDevice == nil {
		log.Printf("🎧 GameBuds: No writable device found for polling - will only listen for unsolicited reports")
		return
	}

	log.Printf("🎧 GameBuds: Starting active polling on device %d (%s) every %v", pollDevice.number, pollDevice.path, gameBudsPollInterval)

	go func() {
		ticker := time.NewTicker(gameBudsPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				log.Printf("🎧 GameBuds: Stopping polling goroutine")
				return
			case <-ticker.C:
				// Request battery status (most important)
				// Note: If device returns all zeros, it doesn't support feature reports
				data, err := m.requestFeatureReport(pollDevice.fd, 0xB7)
				if err != nil {
					// Log only occasionally to avoid spam
					if time.Now().Unix()%30 == 0 {
						log.Printf("🎧 GameBuds: Polling feature report 0xB7 failed: %v", err)
					}
				} else if len(data) > 0 && data[0] == 0xB7 {
					// Check if we got meaningful data (not all zeros)
					allZeros := true
					for _, b := range data {
						if b != 0 {
							allZeros = false
							break
						}
					}
					if !allZeros {
						log.Printf("🎧 GameBuds: Polled battery report: %x", data)
						m.protocol.ParseReport(data)
					}
					// Silently ignore all-zero responses (device doesn't support feature reports)
				}

				// Also request wear status and ANC mode less frequently
				if time.Now().Unix()%3 == 0 {
					// Request wear status
					data, err := m.requestFeatureReport(pollDevice.fd, 0xB5)
					if err == nil && len(data) > 0 && data[0] == 0xB5 {
						allZeros := true
						for _, b := range data {
							if b != 0 {
								allZeros = false
								break
							}
						}
						if !allZeros {
							log.Printf("🎧 GameBuds: Polled wear status report: %x", data)
							m.protocol.ParseReport(data)
						}
					}

					// Request ANC mode
					data, err = m.requestFeatureReport(pollDevice.fd, 0xBD)
					if err == nil && len(data) > 0 && data[0] == 0xBD {
						allZeros := true
						for _, b := range data {
							if b != 0 {
								allZeros = false
								break
							}
						}
						if !allZeros {
							log.Printf("🎧 GameBuds: Polled ANC mode report: %x", data)
							m.protocol.ParseReport(data)
						}
					}
				}
			}
		}
	}()
}

// Close closes all devices
func (m *HIDRawManager) Close() error {
	m.Stop()
	for _, devInfo := range m.devices {
		devInfo.file.Close()
	}
	return nil
}
