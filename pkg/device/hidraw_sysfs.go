package device

import (
	"fmt"
	"path/filepath"
	"strings"
)

const hidrawSysfsDir = "/sys/class/hidraw"

type hidrawSysfsDevice struct {
	Name            string
	Path            string
	InterfaceNumber string
}

func findHIDRawDevices(fs FileSystem, vendorID, productID int) ([]hidrawSysfsDevice, error) {
	files, err := fs.ReadDir(hidrawSysfsDir)
	if err != nil {
		return nil, err
	}

	hidID := hidrawHIDID(vendorID, productID)
	matches := make([]hidrawSysfsDevice, 0)
	for _, f := range files {
		ueventPath := fmt.Sprintf("%s/%s/device/uevent", hidrawSysfsDir, f.Name())
		data, err := fs.ReadFile(ueventPath)
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(string(data)), hidID) {
			continue
		}

		matches = append(matches, hidrawSysfsDevice{
			Name:            f.Name(),
			Path:            filepath.Join("/dev", f.Name()),
			InterfaceNumber: readHIDRawInterfaceNumber(fs, f.Name()),
		})
	}

	return matches, nil
}

func hidrawHIDID(vendorID, productID int) string {
	return fmt.Sprintf("HID_ID=0003:%08X:%08X", vendorID, productID)
}

func readHIDRawInterfaceNumber(fs FileSystem, hidrawName string) string {
	interfacePath := fmt.Sprintf("%s/%s/device/../bInterfaceNumber", hidrawSysfsDir, hidrawName)
	if data, err := fs.ReadFile(interfacePath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "unknown"
}
