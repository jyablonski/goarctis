package device

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
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

	gameBudsPollInterval = 5 * time.Second
)

func _IOC(dir, typ, nr, size uint) uint {
	return (dir << 30) | (typ << 8) | nr | (size << 16)
}

func _IOR(typ, nr, size uint) uint {
	return _IOC(2, typ, nr, size)
}

func _IOW(typ, nr, size uint) uint {
	return _IOC(1, typ, nr, size)
}

func _IOWR(typ, nr, size uint) uint {
	return _IOC(3, typ, nr, size)
}

func HIDIOCGFEATURE(length uint) uint {
	return _IOWR('H', _HIDIOCGFEATURE, length)
}

func HIDIOCSFEATURE(length uint) uint {
	return _IOWR('H', _HIDIOCSFEATURE, length)
}

type FileSystem interface {
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	OpenFile(name string, flag int, perm os.FileMode) (io.ReadCloser, error)
}

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
	fd     *os.File
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

func NewHIDRawManagerWithFS(fs FileSystem) *HIDRawManager {
	return &HIDRawManager{
		protocol:   protocol.NewHandler(),
		stopChan:   make(chan struct{}),
		fs:         fs,
		deviceID:   "steelseries_gamebuds",
		deviceName: "SteelSeries Arctis GameBuds",
	}
}

func (m *HIDRawManager) FindDevices() error {
	matches, err := findHIDRawDevices(m.fs, VendorID, ProductID)
	if err != nil {
		return fmt.Errorf("failed to read hidraw devices: %w", err)
	}

	hidrawPaths := make([]string, 0, len(matches))
	for _, match := range matches {
		hidrawPaths = append(hidrawPaths, match.Path)
		log.Printf("Found GameBuds HID interface: %s (interface %s)", match.Path, match.InterfaceNumber)
	}

	for _, match := range matches {
		if match.InterfaceNumber == "03" {
			log.Printf("Interface %s (%s) is typically the status report interface", match.InterfaceNumber, match.Path)
		}
	}

	if len(hidrawPaths) == 0 {
		return fmt.Errorf("no GameBuds hidraw devices found")
	}

	// Note: On some systems (like Arch Linux), devices may need to be opened in read-write mode
	// to receive certain HID reports. Try read-write first, fall back to read-only if that fails.
	for i, path := range hidrawPaths {
		var f io.ReadCloser
		var err error

		f, err = m.fs.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
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

func (m *HIDRawManager) GetID() string {
	return m.deviceID
}

func (m *HIDRawManager) GetName() string {
	return m.deviceName
}

func (m *HIDRawManager) GetType() DeviceType {
	return DeviceTypeSteelSeriesGameBuds
}

func (m *HIDRawManager) SetOnStateChange(callback func(protocol.DeviceState)) {
	m.onChange = callback
	m.protocol.SetOnChange(func(state protocol.DeviceState) {
		state.DeviceID = m.deviceID
		state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
		if m.onChange != nil {
			m.onChange(state)
		}
	})
}

func (m *HIDRawManager) Start() error {
	if len(m.devices) == 0 {
		return fmt.Errorf("no devices to monitor")
	}

	log.Printf("Monitoring %d HID interfaces...", len(m.devices))

	// This might be required on some systems (like Arch Linux) to start receiving reports
	m.requestInitialFeatureReports()

	initialState := m.GetState()
	log.Printf("🎧 GameBuds: Sending initial state update (DeviceID: %s, DeviceType: %s, IsConnected: %v)",
		initialState.DeviceID, initialState.DeviceType, initialState.IsConnected)
	if m.protocol != nil {
		state := m.protocol.GetState()
		state.DeviceID = m.deviceID
		state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
		if m.onChange != nil {
			m.onChange(state)
		}
	}

	m.startPollingGoroutine()

	for _, deviceInfo := range m.devices {
		info := deviceInfo

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

func (m *HIDRawManager) GetState() protocol.DeviceState {
	state := m.protocol.GetState()
	state.DeviceID = m.deviceID
	state.DeviceType = string(DeviceTypeSteelSeriesGameBuds)
	return state
}

func (m *HIDRawManager) IsConnected() bool {
	return len(m.devices) > 0
}

func (m *HIDRawManager) Stop() error {
	select {
	case <-m.stopChan:
	default:
		close(m.stopChan)
	}
	return nil
}

func (m *HIDRawManager) requestFeatureReport(fd *os.File, reportID byte) ([]byte, error) {
	if fd == nil {
		return nil, fmt.Errorf("file descriptor not available")
	}

	buf := make([]byte, 64)
	buf[0] = reportID

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

// This may be required to "wake up" the device on some systems
func (m *HIDRawManager) requestInitialFeatureReports() {
	for _, devInfo := range m.devices {
		if devInfo.fd != nil {
			log.Printf("🎧 GameBuds: Attempting to request initial feature reports from device %d", devInfo.number)

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
						if len(data) > 0 && data[0] == reportID {
							m.protocol.ParseReport(data)
						}
					}
				}
			}

			log.Printf("🎧 GameBuds: Note: Device appears to use INPUT reports (unsolicited), not FEATURE reports")
			log.Printf("🎧 GameBuds: Polling will continue but may not return data - waiting for device to send input reports")
			break
		}
	}
}

// This implements active polling similar to how Razer devices work
func (m *HIDRawManager) startPollingGoroutine() {
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
				// Note: If device returns all zeros, it doesn't support feature reports
				data, err := m.requestFeatureReport(pollDevice.fd, 0xB7)
				if err != nil {
					if time.Now().Unix()%30 == 0 {
						log.Printf("🎧 GameBuds: Polling feature report 0xB7 failed: %v", err)
					}
				} else if len(data) > 0 && data[0] == 0xB7 {
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
				}

				if time.Now().Unix()%3 == 0 {
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

func (m *HIDRawManager) Close() error {
	m.Stop()
	for _, devInfo := range m.devices {
		devInfo.file.Close()
	}
	return nil
}
