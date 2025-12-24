package device

import (
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

// RazerDevice represents a Razer device monitored via OpenRazer D-Bus
type RazerDevice struct {
	conn         *dbus.Conn
	devicePath   dbus.ObjectPath
	deviceSerial string
	deviceName   string
	state        protocol.DeviceState
	stopChan     chan struct{}
	onChange     func(protocol.DeviceState)
	mu           sync.RWMutex
}

// NewRazerDevice creates a new Razer device monitor
func NewRazerDevice(devicePath dbus.ObjectPath, deviceSerial string) (*RazerDevice, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}

	// Get device name (try getDeviceName, fallback to serial)
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
			DeviceType:  string(DeviceTypeRazerDeathAdder),
			IsConnected: true,
		},
		stopChan: make(chan struct{}),
	}

	// Initial state fetch
	if err := rd.updateState(); err != nil {
		log.Printf("Warning: Failed to fetch initial state for %s: %v", deviceName, err)
	}

	return rd, nil
}

// GetID returns the device serial number
func (r *RazerDevice) GetID() string {
	return r.deviceSerial
}

// GetName returns the device name
func (r *RazerDevice) GetName() string {
	return r.deviceName
}

// GetType returns the device type
func (r *RazerDevice) GetType() DeviceType {
	return DeviceTypeRazerDeathAdder
}

// GetState returns the current device state
func (r *RazerDevice) GetState() protocol.DeviceState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// IsConnected returns whether the device is connected
func (r *RazerDevice) IsConnected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.IsConnected
}

// SetOnStateChange sets the callback for state changes
func (r *RazerDevice) SetOnStateChange(callback func(protocol.DeviceState)) {
	r.mu.Lock()
	r.onChange = callback
	r.mu.Unlock()
}

// Start begins monitoring the device
func (r *RazerDevice) Start() error {
	log.Printf("Starting Razer device monitoring for %s", r.deviceName)
	go r.pollLoop()
	return nil
}

// Stop stops monitoring the device
func (r *RazerDevice) Stop() error {
	select {
	case <-r.stopChan:
		// Already closed
	default:
		close(r.stopChan)
	}
	return nil
}

// Close releases resources
func (r *RazerDevice) Close() error {
	r.Stop()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
	return nil
}

// pollLoop periodically polls the device for battery status
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

				// Check if connection was closed - try to reconnect
				if isConnectionClosed(err) {
					log.Printf("D-Bus connection closed (attempt %d/%d), attempting to reconnect...", consecutiveErrors, maxConsecutiveErrors)

					// Wait longer before reconnecting (device might be switching modes)
					// Longer wait for first few attempts, then shorter
					waitTime := time.Duration(consecutiveErrors) * 500 * time.Millisecond
					if waitTime > 2*time.Second {
						waitTime = 2 * time.Second
					}
					time.Sleep(waitTime)

					if reconnectErr := r.reconnectWithRetry(); reconnectErr != nil {
						log.Printf("Failed to reconnect after retries: %v", reconnectErr)

						// If multiple failures, try restarting OpenRazer daemon
						if consecutiveErrors >= maxConsecutiveErrors {
							log.Printf("Multiple reconnection failures, attempting to restart OpenRazer daemon...")
							if restartErr := r.restartOpenRazerDaemon(); restartErr != nil {
								log.Printf("Failed to restart OpenRazer daemon: %v", restartErr)
							} else {
								log.Printf("OpenRazer daemon restarted, waiting before retry...")
								time.Sleep(2 * time.Second)
								consecutiveErrors = 0 // Reset counter after restart
							}
						}

						// Mark as disconnected
						r.mu.Lock()
						oldState := r.state
						r.state.IsConnected = false
						if r.onChange != nil && oldState != r.state {
							r.onChange(r.state)
						}
						r.mu.Unlock()
					} else {
						log.Printf("Successfully reconnected to Razer device")

						// Wait a bit for device to be ready after mode switch
						time.Sleep(1 * time.Second)

						// Try to update state after reconnection
						if err := r.updateState(); err != nil {
							// If update still fails after reconnection, check if it's still a connection error
							if isConnectionClosed(err) {
								log.Printf("Connection still closed after reconnect, will retry on next poll")
								// Don't reset consecutiveErrors - let it accumulate
							} else {
								log.Printf("Error updating state after reconnect: %v", err)
								// Reset on non-connection errors (device might be working now)
								consecutiveErrors = 0
							}
						} else {
							// Success - reset error counter
							consecutiveErrors = 0
						}
					}
				} else {
					log.Printf("Error updating Razer device state: %v", err)
					// Mark as disconnected if we can't communicate
					r.mu.Lock()
					oldState := r.state
					r.state.IsConnected = false
					if r.onChange != nil && oldState != r.state {
						r.onChange(r.state)
					}
					r.mu.Unlock()
				}
			} else {
				// Success - reset error counter
				consecutiveErrors = 0
			}
		}
	}
}

// isConnectionClosed checks if the error indicates a closed connection
func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for connection closed errors (exact match or contains, for wrapped errors)
	return errStr == "dbus: connection closed by user" ||
		errStr == "dbus: connection closed" ||
		errStr == "EOF" ||
		errStr == "use of closed network connection" ||
		// Check if error message contains these strings (for wrapped errors like "failed to get battery: dbus: connection closed by user")
		contains(errStr, "connection closed by user") ||
		contains(errStr, "dbus: connection closed") ||
		contains(errStr, "use of closed network connection")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		strings.Contains(s, substr))
}

// reconnectWithRetry re-establishes the D-Bus connection with retries
func (r *RazerDevice) reconnectWithRetry() error {
	maxRetries := 5 // Increased retries for mode switching
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

		// Wait a bit before verifying (device might still be switching modes)
		time.Sleep(500 * time.Millisecond)

		// Verify device is still accessible after reconnection
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
	return fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

// reconnect re-establishes the D-Bus connection
func (r *RazerDevice) reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close old connection if it exists
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}

	// Create new connection
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("failed to reconnect to session D-Bus: %w", err)
	}

	r.conn = conn
	return nil
}

// verifyDevice checks if the device is still accessible on D-Bus
func (r *RazerDevice) verifyDevice() error {
	r.mu.RLock()
	conn := r.conn
	devicePath := r.devicePath
	r.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection not available")
	}

	obj := conn.Object(razerService, devicePath)
	// Try a simple call to verify device exists
	var battery float64
	err := obj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
	if err != nil {
		return fmt.Errorf("device not accessible: %w", err)
	}
	return nil
}

// restartOpenRazerDaemon attempts to restart the OpenRazer daemon
func (r *RazerDevice) restartOpenRazerDaemon() error {
	// Try to stop the daemon via D-Bus first
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

	// Try to restart via systemd user service
	cmd := exec.Command("systemctl", "--user", "restart", "openrazer-daemon.service")
	if err := cmd.Run(); err != nil {
		// Fallback: try without .service suffix
		cmd = exec.Command("systemctl", "--user", "restart", "openrazer-daemon")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restart OpenRazer daemon: %w", err)
		}
	}

	// Wait for daemon to be ready
	time.Sleep(2 * time.Second)

	// Reconnect after daemon restart
	return r.reconnect()
}

// updateState fetches the current battery and charging status from D-Bus
func (r *RazerDevice) updateState() error {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("D-Bus connection not available")
	}

	obj := conn.Object(razerService, r.devicePath)

	// Get battery level
	var battery float64
	err := obj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
	if err != nil {
		return fmt.Errorf("failed to get battery: %w", err)
	}

	// Get charging status
	var isCharging bool
	err = obj.Call(razerPowerIface+".isCharging", 0).Store(&isCharging)
	if err != nil {
		// When not charging, isCharging might return false or error
		// In wireless mode, isCharging should be false
		// We'll treat errors as "not charging" (wireless mode)
		isCharging = false
	}

	batteryInt := int(battery)

	r.mu.Lock()
	oldState := r.state
	r.state.Battery = &batteryInt
	r.state.IsCharging = &isCharging
	r.state.IsConnected = true
	r.mu.Unlock()

	// Trigger callback if state changed
	if r.onChange != nil && oldState != r.state {
		r.mu.RLock()
		currentState := r.state
		r.mu.RUnlock()
		r.onChange(currentState)
	}

	log.Printf("🖱️ Razer %s: Battery %d%% (Charging: %v)", r.deviceName, batteryInt, isCharging)
	return nil
}

// DiscoverRazerDevices discovers all Razer devices with battery support via OpenRazer
func DiscoverRazerDevices() ([]*RazerDevice, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session D-Bus: %w", err)
	}
	defer conn.Close()

	// Check if OpenRazer daemon is running
	obj := conn.Object(razerService, razerManagerPath)
	var devices []string
	err = obj.Call(razerManagerIface+".getDevices", 0).Store(&devices)
	if err != nil {
		return nil, fmt.Errorf("OpenRazer daemon not available or error: %w", err)
	}

	if len(devices) == 0 {
		return []*RazerDevice{}, nil
	}

	var razerDevices []*RazerDevice
	for _, deviceSerial := range devices {
		// Device path format: /org/razer/device/{serial}
		devicePath := dbus.ObjectPath(fmt.Sprintf("/org/razer/device/%s", deviceSerial))
		deviceObj := conn.Object(razerService, devicePath)

		// Try to get battery to check if device supports it
		// If this fails, the device doesn't support battery monitoring
		var battery float64
		err := deviceObj.Call(razerPowerIface+".getBattery", 0).Store(&battery)
		if err != nil {
			// Device doesn't support battery, skip it
			log.Printf("Device %s doesn't support battery monitoring: %v", deviceSerial, err)
			continue
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
