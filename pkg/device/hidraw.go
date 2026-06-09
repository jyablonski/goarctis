package device

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

const (
	VendorID  = 0x1038
	ProductID = 0x230a

	// CaseProductID is the charging case, a separate USB device that only
	// enumerates while it is plugged into USB. Its FFC0 control interface
	// (interface 00) answers a 0xB7 query with the case battery in byte 3.
	CaseProductID        = 0x230c
	caseControlInterface = "00"

	// statusQueryCommand is written as a 64-byte OUTPUT report on the FFC0
	// control interface (interface 03). The dongle replies with a 0xB0 INPUT
	// report containing battery and wear state, which the read goroutine parses.
	// The device uses unnumbered reports, so byte 0 is the report number (0) and
	// the command goes in byte 1.
	statusQueryCommand = 0xB0

	// caseBatteryCommand queries the charging case; byte 3 of the reply is the
	// case battery percentage.
	caseBatteryCommand = 0xB7

	gameBudsPollInterval = 5 * time.Second
)

var (
	ErrNoGameBudsHIDRawDevices = errors.New("no GameBuds hidraw devices found")
	ErrNoHIDRawDevicesOpened   = errors.New("could not open any hidraw devices")
	ErrNoDevicesToMonitor      = errors.New("no devices to monitor")
	ErrFileDescriptorMissing   = errors.New("file descriptor not available")
	ErrStatusQueryFailed       = errors.New("status query write failed")
)

type FileSystem interface {
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	OpenFile(name string, flag int, perm os.FileMode) (io.ReadCloser, error)
}

type RealFileSystem struct{}

func (fs RealFileSystem) ReadDir(dirname string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(dirname)
	if err != nil {
		return nil, err
	}

	infos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (fs RealFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
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
	caseDevice *deviceInfo
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
		return ErrNoGameBudsHIDRawDevices
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
		return ErrNoHIDRawDevicesOpened
	}

	log.Printf("Successfully opened %d HID interfaces", len(m.devices))

	m.findCaseDevice()

	return nil
}

// findCaseDevice locates and opens the charging case's control interface, if
// the case is currently plugged into USB. The case is optional: failure to find
// it is not an error (it simply means case battery is unavailable).
func (m *HIDRawManager) findCaseDevice() {
	matches, err := findHIDRawDevices(m.fs, VendorID, CaseProductID)
	if err != nil {
		log.Printf("🎧 GameBuds: case lookup failed: %v", err)
		return
	}

	for _, match := range matches {
		if match.InterfaceNumber != caseControlInterface {
			continue
		}

		f, err := m.fs.OpenFile(match.Path, os.O_RDWR, 0)
		if err != nil {
			log.Printf("🎧 GameBuds: could not open case control interface %s: %v", match.Path, err)
			return
		}

		fd, _ := f.(*os.File)
		m.caseDevice = &deviceInfo{file: f, path: match.Path, fd: fd}
		log.Printf("🎧 GameBuds: found charging case control interface at %s", match.Path)
		return
	}
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
		return ErrNoDevicesToMonitor
	}

	log.Printf("Monitoring %d HID interfaces...", len(m.devices))

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
						m.parseReport(data)
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

	// Start polling after the read goroutines so their replies are picked up.
	m.startPollingGoroutine()
	m.startCasePolling()

	return nil
}

func (m *HIDRawManager) parseReport(data []byte) {
	if err := m.protocol.ParseReport(data); err != nil {
		log.Printf("🎧 GameBuds: failed to parse HID report: %v", err)
	}
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

// controlDevice returns the first writable interface (the FFC0 control channel
// on interface 03), which is where status queries are written and from which
// the dongle's status reports are read.
func (m *HIDRawManager) controlDevice() *deviceInfo {
	for i := range m.devices {
		if m.devices[i].fd != nil {
			return &m.devices[i]
		}
	}
	return nil
}

// writeStatusQuery writes the 0xB0 status command to the control interface. The
// dongle replies with a 0xB0 input report that the read goroutine parses. The
// device occasionally NAKs a write (ETIMEDOUT/EAGAIN), so retry a few times.
func (m *HIDRawManager) writeStatusQuery(fd *os.File) error {
	if fd == nil {
		return ErrFileDescriptorMissing
	}

	buf := make([]byte, 64)
	buf[0] = 0x00 // report number (device uses unnumbered reports)
	buf[1] = statusQueryCommand

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := fd.Write(buf); err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("%w: %v", ErrStatusQueryFailed, lastErr)
}

// startPollingGoroutine periodically asks the dongle for a status report by
// writing the 0xB0 command to the control interface. The reply arrives as an
// input report and is handled by the read goroutine started in Start.
func (m *HIDRawManager) startPollingGoroutine() {
	pollDevice := m.controlDevice()
	if pollDevice == nil {
		log.Printf("🎧 GameBuds: No writable device found - cannot poll for status")
		return
	}

	log.Printf("🎧 GameBuds: Polling status on device %d (%s) every %v", pollDevice.number, pollDevice.path, gameBudsPollInterval)

	// Send an immediate query so the tray populates without waiting a full tick.
	if err := m.writeStatusQuery(pollDevice.fd); err != nil {
		log.Printf("🎧 GameBuds: Initial status query failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(gameBudsPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				log.Printf("🎧 GameBuds: Stopping polling goroutine")
				return
			case <-ticker.C:
				if err := m.writeStatusQuery(pollDevice.fd); err != nil {
					log.Printf("🎧 GameBuds: Status query failed: %v", err)
				}
			}
		}
	}()
}

// startCasePolling polls the charging case (when plugged in) for its battery
// level. Unlike the dongle, the case answers synchronously: we write the 0xB7
// query and read the reply on the same goroutine, since the case interface does
// not emit unsolicited reports.
func (m *HIDRawManager) startCasePolling() {
	if m.caseDevice == nil || m.caseDevice.fd == nil {
		return
	}

	fd := m.caseDevice.fd
	log.Printf("🎧 GameBuds: Polling charging case battery on %s every %v", m.caseDevice.path, gameBudsPollInterval)

	m.queryCaseBattery(fd)

	go func() {
		ticker := time.NewTicker(gameBudsPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				log.Printf("🎧 GameBuds: Stopping case polling goroutine")
				return
			case <-ticker.C:
				m.queryCaseBattery(fd)
			}
		}
	}()
}

// queryCaseBattery writes the case battery query and reads the reply. The case
// reply is b7 00 00 <battery> ..., so byte 3 carries the percentage.
func (m *HIDRawManager) queryCaseBattery(fd *os.File) {
	buf := make([]byte, 64)
	buf[0] = 0x00
	buf[1] = caseBatteryCommand

	var writeErr error
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := fd.Write(buf); err != nil {
			writeErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		writeErr = nil
		break
	}
	if writeErr != nil {
		log.Printf("🎧 GameBuds: case battery query failed: %v", writeErr)
		return
	}

	reply := make([]byte, 64)
	n, err := fd.Read(reply)
	if err != nil {
		if err != io.EOF {
			log.Printf("🎧 GameBuds: case battery read failed: %v", err)
		}
		return
	}

	if n >= 4 && reply[0] == caseBatteryCommand {
		battery := int(reply[3])
		log.Printf("🔋 GameBuds case battery: %d%%", battery)
		m.protocol.SetDockBattery(battery)
	}
}

func (m *HIDRawManager) Close() error {
	var errs []error
	if err := m.Stop(); err != nil {
		errs = append(errs, err)
	}
	for _, devInfo := range m.devices {
		if err := devInfo.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", devInfo.path, err))
		}
	}
	if m.caseDevice != nil && m.caseDevice.file != nil {
		if err := m.caseDevice.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", m.caseDevice.path, err))
		}
	}
	return errors.Join(errs...)
}
