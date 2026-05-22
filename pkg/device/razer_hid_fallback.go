package device

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

const (
	razerHIDSysfsDir = "/sys/bus/hid/devices"
	razerVendorID    = "00001532"
)

type razerHIDDevice struct {
	ID   string
	Name string
}

func newRazerWarningDevice(id, name string) *RazerDevice {
	if name == "" {
		name = "Razer Device"
	}
	return &RazerDevice{
		deviceSerial: id,
		deviceName:   name,
		stopChan:     make(chan struct{}),
		warningOnly:  true,
		state: protocol.DeviceState{
			DeviceID:    id,
			DeviceType:  string(DeviceTypeRazer),
			IsConnected: true,
			Warning:     razerBatteryUnavailableWarning,
		},
	}
}

func discoverRazerWarningDevices(fs FileSystem) []*RazerDevice {
	hidDevices, err := findRazerHIDDevices(fs)
	if err != nil {
		log.Printf("Could not scan for Razer HID devices: %v", err)
		return nil
	}

	devices := make([]*RazerDevice, 0, len(hidDevices))
	for _, hidDevice := range hidDevices {
		devices = append(devices, newRazerWarningDevice(hidDevice.ID, hidDevice.Name))
	}
	return devices
}

func findRazerHIDDevices(fs FileSystem) ([]razerHIDDevice, error) {
	entries, err := fs.ReadDir(razerHIDSysfsDir)
	if err != nil {
		return nil, err
	}

	var devices []razerHIDDevice
	seen := make(map[string]struct{})
	for _, entry := range entries {
		deviceDir := filepath.Join(razerHIDSysfsDir, entry.Name())
		uevent, err := fs.ReadFile(filepath.Join(deviceDir, "uevent"))
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(string(uevent)), ":"+razerVendorID+":") {
			continue
		}

		name := parseUeventValue(uevent, "HID_NAME")
		if name == "" {
			name = readTrimmedFile(fs, filepath.Join(deviceDir, "name"))
		}
		uniqueID := parseUeventValue(uevent, "HID_UNIQ")
		if uniqueID == "" {
			uniqueID = parseUeventValue(uevent, "HID_ID")
		}
		if uniqueID == "" {
			uniqueID = entry.Name()
		}
		id := "razer-hid-" + sanitizeDeviceID(uniqueID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		devices = append(devices, razerHIDDevice{ID: id, Name: name})
	}

	return devices, nil
}

func parseUeventValue(data []byte, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func sanitizeDeviceID(id string) string {
	replacer := strings.NewReplacer(":", "-", ".", "-", " ", "-")
	return replacer.Replace(strings.TrimSpace(id))
}

func readTrimmedFile(fs FileSystem, path string) string {
	data, err := fs.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
