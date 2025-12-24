package device

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

// MockFileInfo implements os.FileInfo
type MockFileInfo struct {
	name string
}

func (m MockFileInfo) Name() string       { return m.name }
func (m MockFileInfo) Size() int64        { return 0 }
func (m MockFileInfo) Mode() os.FileMode  { return 0 }
func (m MockFileInfo) ModTime() time.Time { return time.Time{} }
func (m MockFileInfo) IsDir() bool        { return false }
func (m MockFileInfo) Sys() interface{}   { return nil }

// MockFileSystem implements FileSystem for testing
type MockFileSystem struct {
	files       map[string][]byte
	dirContents map[string][]os.FileInfo
	openError   error
}

func (m *MockFileSystem) ReadDir(dirname string) ([]os.FileInfo, error) {
	if contents, ok := m.dirContents[dirname]; ok {
		return contents, nil
	}
	return nil, fmt.Errorf("directory not found")
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if data, ok := m.files[filename]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("file not found")
}

func (m *MockFileSystem) OpenFile(name string, flag int, perm os.FileMode) (io.ReadCloser, error) {
	if m.openError != nil {
		return nil, m.openError
	}
	if data, ok := m.files[name]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, fmt.Errorf("file not found")
}

func TestNewHIDRawManager(t *testing.T) {
	manager := NewHIDRawManager()

	if manager == nil {
		t.Fatal("NewHIDRawManager returned nil")
	}

	if manager.protocol == nil {
		t.Error("Protocol handler is nil")
	}

	if manager.stopChan == nil {
		t.Error("Stop channel is nil")
	}

	if manager.fs == nil {
		t.Error("FileSystem is nil")
	}
}

func TestFindDevices_Success(t *testing.T) {
	mockFS := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {
				MockFileInfo{name: "hidraw0"},
				MockFileInfo{name: "hidraw1"},
			},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent":              []byte("HID_ID=0003:00001038:0000230A\n"),
			"/sys/class/hidraw/hidraw0/device/../bInterfaceNumber": []byte("03\n"),
			"/sys/class/hidraw/hidraw1/device/uevent":              []byte("HID_ID=0003:00001038:0000230A\n"),
			"/sys/class/hidraw/hidraw1/device/../bInterfaceNumber": []byte("04\n"),
			"/dev/hidraw0": []byte{},
			"/dev/hidraw1": []byte{},
		},
	}

	manager := NewHIDRawManagerWithFS(mockFS)
	err := manager.FindDevices()

	if err != nil {
		t.Fatalf("FindDevices failed: %v", err)
	}

	if len(manager.devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(manager.devices))
	}
}

func TestFindDevices_NoDevices(t *testing.T) {
	mockFS := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {},
		},
		files: map[string][]byte{},
	}

	manager := NewHIDRawManagerWithFS(mockFS)
	err := manager.FindDevices()

	if err == nil {
		t.Error("Expected error when no devices found")
	}

	expectedError := "no GameBuds hidraw devices found"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestFindDevices_WrongVendorID(t *testing.T) {
	mockFS := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {
				MockFileInfo{name: "hidraw0"},
			},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent": []byte("HID_ID=0003:00001234:00005678\n"),
		},
	}

	manager := NewHIDRawManagerWithFS(mockFS)
	err := manager.FindDevices()

	if err == nil {
		t.Error("Expected error when wrong vendor ID")
	}
}

func TestFindDevices_CannotOpenDevice(t *testing.T) {
	mockFS := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {
				MockFileInfo{name: "hidraw0"},
			},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent":              []byte("HID_ID=0003:00001038:0000230A\n"),
			"/sys/class/hidraw/hidraw0/device/../bInterfaceNumber": []byte("03\n"),
		},
		openError: fmt.Errorf("permission denied"),
	}

	manager := NewHIDRawManagerWithFS(mockFS)
	err := manager.FindDevices()

	if err == nil {
		t.Error("Expected error when cannot open devices")
	}

	expectedError := "could not open any hidraw devices"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestSetOnStateChange(t *testing.T) {
	manager := NewHIDRawManager()
	callbackCalled := false

	manager.SetOnStateChange(func(state protocol.DeviceState) {
		callbackCalled = true
	})

	// Simulate a state change
	manager.protocol.ParseReport([]byte{0xB7, 75, 80})

	if !callbackCalled {
		t.Error("Callback was not called")
	}
}

func TestGetState(t *testing.T) {
	manager := NewHIDRawManager()

	// Set some state
	manager.protocol.ParseReport([]byte{0xB7, 75, 80})
	manager.protocol.ParseReport([]byte{0xBD, 0x01})

	state := manager.GetState()

	if state.LeftBattery == nil || *state.LeftBattery != 75 {
		val := 0
		if state.LeftBattery != nil {
			val = *state.LeftBattery
		}
		t.Errorf("Left battery = %d, want 75", val)
	}
	if state.RightBattery == nil || *state.RightBattery != 80 {
		val := 0
		if state.RightBattery != nil {
			val = *state.RightBattery
		}
		t.Errorf("Right battery = %d, want 80", val)
	}
	if state.ANCMode == nil || *state.ANCMode != protocol.ANCTransparency {
		t.Errorf("ANC mode = %v, want Transparency", state.ANCMode)
	}
}

func TestStopAndClose(t *testing.T) {
	mockFS := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {
				MockFileInfo{name: "hidraw0"},
			},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent": []byte("HID_ID=0003:00001038:0000230A\n"),
			"/dev/hidraw0": []byte{},
		},
	}

	manager := NewHIDRawManagerWithFS(mockFS)
	err := manager.FindDevices()
	if err != nil {
		t.Fatalf("FindDevices failed: %v", err)
	}

	// Close should not panic
	if err := manager.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify stop channel is closed
	select {
	case <-manager.stopChan:
		// Channel is closed, good
	case <-time.After(100 * time.Millisecond):
		t.Error("Stop channel was not closed")
	}
}

func TestStart_NoDevices(t *testing.T) {
	manager := NewHIDRawManager()

	// Should return error when no devices
	err := manager.Start()
	if err == nil {
		t.Error("Start should return error when no devices")
	}
}
