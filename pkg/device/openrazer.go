package device

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/jyablonski/goarctis/pkg/protocol"
)

const (
	razerService      = "org.razer"
	razerManagerPath  = "/org/razer"
	razerManagerIface = "razer.devices"
	razerDeviceIface  = "razer.device"
	razerPowerIface   = "razer.device.power"
	pollInterval      = 5 * time.Second
)

var (
	ErrRazerConnectionUnavailable = errors.New("razer D-Bus connection not available")
	ErrRazerReconnectFailed       = errors.New("failed to reconnect to Razer device")
	ErrOpenRazerDaemonUnavailable = errors.New("OpenRazer daemon not available")
)

const razerBatteryUnavailableWarning = "Battery unavailable: OpenRazer driver is not reporting battery data"

type RazerDevice struct {
	conn         *dbus.Conn
	devicePath   dbus.ObjectPath
	deviceSerial string
	deviceName   string
	state        protocol.DeviceState
	stopChan     chan struct{}
	onChange     func(protocol.DeviceState)
	mu           sync.RWMutex
	warningOnly  bool
}

func NewRazerDevice(devicePath dbus.ObjectPath, deviceSerial string) (*RazerDevice, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}

	deviceName := fmt.Sprintf("Razer Device (%s)", deviceSerial)
	var name string
	err = conn.Object(razerService, devicePath).Call(razerDeviceIface+".getDeviceName", 0).Store(&name)
	if err == nil && name != "" {
		deviceName = name
	}

	rd := &RazerDevice{
		conn:         conn,
		devicePath:   devicePath,
		deviceSerial: deviceSerial,
		deviceName:   deviceName,
		state: protocol.DeviceState{
			DeviceID:    deviceSerial,
			DeviceType:  string(DeviceTypeRazer),
			IsConnected: true,
		},
		stopChan: make(chan struct{}),
	}

	if err := rd.updateState(); err != nil {
		log.Printf("Warning: Failed to fetch initial state for %s: %v", deviceName, err)
	}

	return rd, nil
}

func (r *RazerDevice) GetID() string {
	return r.deviceSerial
}

func (r *RazerDevice) GetName() string {
	return r.deviceName
}

func (r *RazerDevice) GetType() DeviceType {
	return DeviceTypeRazer
}

func (r *RazerDevice) GetState() protocol.DeviceState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *RazerDevice) IsConnected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.IsConnected
}

func (r *RazerDevice) SetOnStateChange(callback func(protocol.DeviceState)) {
	r.mu.Lock()
	r.onChange = callback
	r.mu.Unlock()
}

func (r *RazerDevice) emitCurrentState() {
	r.mu.RLock()
	currentState := r.state
	onChange := r.onChange
	r.mu.RUnlock()

	if onChange != nil {
		onChange(currentState)
	}
}

func (r *RazerDevice) setState(update func(*protocol.DeviceState)) {
	r.mu.Lock()
	oldState := r.state
	update(&r.state)
	currentState := r.state
	onChange := r.onChange
	changed := !oldState.Equal(currentState)
	r.mu.Unlock()

	if changed && onChange != nil {
		onChange(currentState)
	}
}

func (r *RazerDevice) setDisconnected() {
	r.setState(func(state *protocol.DeviceState) {
		state.IsConnected = false
	})
}

func (r *RazerDevice) Start() error {
	log.Printf("Starting Razer device monitoring for %s", r.deviceName)
	r.emitCurrentState()
	if r.warningOnly {
		return nil
	}
	go r.pollLoop()
	return nil
}

func (r *RazerDevice) Stop() error {
	select {
	case <-r.stopChan:
	default:
		close(r.stopChan)
	}
	return nil
}

func (r *RazerDevice) Close() error {
	var errs []error
	if err := r.Stop(); err != nil {
		errs = append(errs, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			errs = append(errs, err)
		}
		r.conn = nil
	}
	return errors.Join(errs...)
}

func (r *RazerDevice) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxConsecutiveErrors := 3

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			if err := r.updateState(); err != nil {
				consecutiveErrors++

				if isConnectionClosed(err) {
					log.Printf("D-Bus connection closed (attempt %d/%d), attempting to reconnect...", consecutiveErrors, maxConsecutiveErrors)

					waitTime := time.Duration(consecutiveErrors) * 500 * time.Millisecond
					if waitTime > 2*time.Second {
						waitTime = 2 * time.Second
					}
					time.Sleep(waitTime)

					if reconnectErr := r.reconnectWithRetry(); reconnectErr != nil {
						log.Printf("Failed to reconnect after retries: %v", reconnectErr)

						if consecutiveErrors >= maxConsecutiveErrors {
							log.Printf("Multiple reconnection failures, attempting to restart OpenRazer daemon...")
							if restartErr := r.restartOpenRazerDaemon(); restartErr != nil {
								log.Printf("Failed to restart OpenRazer daemon: %v", restartErr)
							} else {
								log.Printf("OpenRazer daemon restarted, waiting before retry...")
								time.Sleep(2 * time.Second)
								consecutiveErrors = 0
							}
						}

						r.setDisconnected()
					} else {
						log.Printf("Successfully reconnected to Razer device")

						time.Sleep(1 * time.Second)

						if err := r.updateState(); err != nil {
							if isConnectionClosed(err) {
								log.Printf("Connection still closed after reconnect, will retry on next poll")
								// Don't reset consecutiveErrors - let it accumulate
							} else {
								log.Printf("Error updating state after reconnect: %v", err)
								consecutiveErrors = 0
							}
						} else {
							consecutiveErrors = 0
						}
					}
				} else {
					log.Printf("Error updating Razer device state: %v", err)
					r.setDisconnected()
				}
			} else {
				consecutiveErrors = 0
			}
		}
	}
}

func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return errStr == "EOF" ||
		strings.Contains(errStr, "connection closed by user") ||
		strings.Contains(errStr, "dbus: connection closed") ||
		strings.Contains(errStr, "use of closed network connection")
}

func isUnsupportedBatteryMethod(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unknownmethod") ||
		strings.Contains(errStr, "unknown method") ||
		strings.Contains(errStr, "no such interface") ||
		strings.Contains(errStr, "does not exist")
}

func (r *RazerDevice) reconnectWithRetry() error {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if err := r.reconnect(); err != nil {
			if i < maxRetries-1 {
				backoff := time.Duration(i+1) * 500 * time.Millisecond
				log.Printf("Reconnection attempt %d/%d failed, waiting %v...", i+1, maxRetries, backoff)
				time.Sleep(backoff)
				continue
			}
			return err
		}

		time.Sleep(500 * time.Millisecond)

		if err := r.verifyDevice(); err != nil {
			if i < maxRetries-1 {
				log.Printf("Device verification failed, retrying... (attempt %d/%d): %v", i+1, maxRetries, err)
				backoff := time.Duration(i+1) * 500 * time.Millisecond
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("device verification failed after reconnection: %w", err)
		}

		return nil
	}
	return fmt.Errorf("%w after %d attempts", ErrRazerReconnectFailed, maxRetries)
}

func (r *RazerDevice) reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			log.Printf("Error closing stale Razer D-Bus connection: %v", err)
		}
		r.conn = nil
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("failed to reconnect to session D-Bus: %w", err)
	}

	r.conn = conn
	return nil
}

func (r *RazerDevice) verifyDevice() error {
	r.mu.RLock()
	conn := r.conn
	devicePath := r.devicePath
	r.mu.RUnlock()

	if conn == nil {
		return ErrRazerConnectionUnavailable
	}

	obj := conn.Object(razerService, devicePath)
	var battery float64
	err := obj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
	if err != nil {
		if isConnectionClosed(err) {
			return fmt.Errorf("device not accessible: %w", err)
		}
		return nil
	}
	return nil
}

func (r *RazerDevice) restartOpenRazerDaemon() error {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()

	if conn != nil {
		obj := conn.Object(razerService, razerManagerPath)
		err := obj.Call("razer.daemon.stop", 0).Store()
		if err != nil {
			log.Printf("Could not stop daemon via D-Bus: %v", err)
		}
		time.Sleep(1 * time.Second)
	}

	cmd := exec.Command("systemctl", "--user", "restart", "openrazer-daemon.service")
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("systemctl", "--user", "restart", "openrazer-daemon")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restart OpenRazer daemon: %w", err)
		}
	}

	time.Sleep(2 * time.Second)

	return r.reconnect()
}

func (r *RazerDevice) updateState() error {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()

	if conn == nil {
		return ErrRazerConnectionUnavailable
	}

	obj := conn.Object(razerService, r.devicePath)

	var battery float64
	err := obj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
	if err != nil {
		if isConnectionClosed(err) {
			return fmt.Errorf("failed to get battery: %w", err)
		}
		r.setBatteryUnavailable()
		log.Printf("Razer %s: battery unavailable: %v", r.deviceName, err)
		return nil
	}

	var isCharging bool
	err = obj.Call(razerPowerIface+".isCharging", 0).Store(&isCharging)
	if err != nil {
		// When not charging, isCharging might return false or error
		// In wireless mode, isCharging should be false
		// We'll treat errors as "not charging" (wireless mode)
		isCharging = false
	}

	batteryInt := int(battery)

	r.setState(func(state *protocol.DeviceState) {
		state.Battery = &batteryInt
		state.IsCharging = &isCharging
		state.IsConnected = true
		state.Warning = ""
	})

	log.Printf("🖱️ Razer %s: Battery %d%% (Charging: %v)", r.deviceName, batteryInt, isCharging)
	return nil
}

func (r *RazerDevice) setBatteryUnavailable() {
	r.setState(func(state *protocol.DeviceState) {
		state.Battery = nil
		state.IsCharging = nil
		state.IsConnected = true
		state.Warning = razerBatteryUnavailableWarning
	})
}

func DiscoverRazerDevices() ([]*RazerDevice, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		if warningDevices := discoverRazerWarningDevices(RealFileSystem{}); len(warningDevices) > 0 {
			log.Printf("OpenRazer D-Bus unavailable; showing warning for %d Razer HID device(s): %v", len(warningDevices), err)
			return warningDevices, nil
		}
		return nil, fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing Razer discovery D-Bus connection: %v", err)
		}
	}()

	obj := conn.Object(razerService, razerManagerPath)
	var devices []string
	err = obj.Call(razerManagerIface+".getDevices", 0).Store(&devices)
	if err != nil {
		if warningDevices := discoverRazerWarningDevices(RealFileSystem{}); len(warningDevices) > 0 {
			log.Printf("OpenRazer daemon unavailable; showing warning for %d Razer HID device(s): %v", len(warningDevices), err)
			return warningDevices, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrOpenRazerDaemonUnavailable, err)
	}

	if len(devices) == 0 {
		if warningDevices := discoverRazerWarningDevices(RealFileSystem{}); len(warningDevices) > 0 {
			log.Printf("OpenRazer returned no devices; showing warning for %d Razer HID device(s)", len(warningDevices))
			return warningDevices, nil
		}
		return []*RazerDevice{}, nil
	}

	var razerDevices []*RazerDevice
	for _, deviceSerial := range devices {
		devicePath := dbus.ObjectPath(fmt.Sprintf("/org/razer/device/%s", deviceSerial))
		deviceObj := conn.Object(razerService, devicePath)

		var battery float64
		err := deviceObj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
		if err != nil {
			if isConnectionClosed(err) {
				log.Printf("Device %s not accessible during Razer discovery: %v", deviceSerial, err)
				continue
			}
			if isUnsupportedBatteryMethod(err) {
				log.Printf("Device %s doesn't support battery monitoring: %v", deviceSerial, err)
				continue
			}
			log.Printf("Device %s reports no battery data; showing tray warning: %v", deviceSerial, err)
		}

		device, err := NewRazerDevice(devicePath, deviceSerial)
		if err != nil {
			log.Printf("Warning: Failed to create Razer device %s: %v", deviceSerial, err)
			continue
		}

		razerDevices = append(razerDevices, device)
	}

	return razerDevices, nil
}
