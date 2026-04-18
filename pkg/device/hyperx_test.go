package device

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jyablonski/goarctis/pkg/protocol"
)

type scriptedTransport struct {
	mu sync.Mutex

	writes  [][]byte
	reads   []readResult
	readIdx int

	drainCount int
	drainErr   error

	closed bool
}

type readResult struct {
	data []byte
	err  error
}

func (s *scriptedTransport) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := make([]byte, len(p))
	copy(buf, p)
	s.writes = append(s.writes, buf)
	return len(p), nil
}

func (s *scriptedTransport) ReadTimeout(p []byte, _ time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readIdx >= len(s.reads) {
		return 0, errHyperXReadTimeout
	}
	r := s.reads[s.readIdx]
	s.readIdx++
	if r.err != nil {
		return 0, r.err
	}
	n := copy(p, r.data)
	return n, nil
}

func (s *scriptedTransport) Drain() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainCount++
	return s.drainErr
}

func (s *scriptedTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func makeResp(subcmd, dataByte byte) []byte {
	r := make([]byte, hyperxReportSize)
	r[0] = hyperxCmdHeader0
	r[1] = hyperxCmdHeader1
	r[2] = subcmd
	r[3] = dataByte
	return r
}

func newHarness(tr *scriptedTransport) *HyperXDevice {
	d := NewHyperXDeviceWithDeps(nil, func(string) (hidTransport, error) { return tr, nil })
	d.hidrawPath = "/dev/hidraw-test"
	d.transport = tr
	return d
}

func TestHyperX_FindDevice_Success(t *testing.T) {
	fs := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {
				MockFileInfo{name: "hidraw0"},
				MockFileInfo{name: "hidraw7"},
			},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent":              []byte("HID_ID=0003:00001234:00005678\n"),
			"/sys/class/hidraw/hidraw7/device/uevent":              []byte("HID_ID=0003:000003F0:0000098D\n"),
			"/sys/class/hidraw/hidraw7/device/../bInterfaceNumber": []byte("03\n"),
		},
	}

	d := NewHyperXDeviceWithDeps(fs, openRealHIDTransport)
	if err := d.FindDevice(); err != nil {
		t.Fatalf("FindDevice: %v", err)
	}
	if d.hidrawPath != "/dev/hidraw7" {
		t.Errorf("hidrawPath = %q, want /dev/hidraw7", d.hidrawPath)
	}
	if d.interfaceNum != "03" {
		t.Errorf("interfaceNum = %q, want 03", d.interfaceNum)
	}
}

func TestHyperX_FindDevice_NotFound(t *testing.T) {
	fs := &MockFileSystem{
		dirContents: map[string][]os.FileInfo{
			"/sys/class/hidraw": {MockFileInfo{name: "hidraw0"}},
		},
		files: map[string][]byte{
			"/sys/class/hidraw/hidraw0/device/uevent": []byte("HID_ID=0003:00001234:00005678\n"),
		},
	}

	d := NewHyperXDeviceWithDeps(fs, openRealHIDTransport)
	if err := d.FindDevice(); err == nil {
		t.Fatal("expected error for missing HyperX device")
	}
}

func TestHyperX_PollOnce_Normal(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{
			{data: makeResp(hyperxSubOnline, 0x00)},
			{data: makeResp(hyperxSubCharge, 0x00)},
			{data: makeResp(hyperxSubBattery, 0x55)},
		},
	}
	d := newHarness(tr)

	var cbState protocol.DeviceState
	var cbFired int
	d.SetOnStateChange(func(s protocol.DeviceState) {
		cbFired++
		cbState = s
	})

	d.pollOnce()

	if tr.drainCount != 1 {
		t.Errorf("drainCount = %d, want 1", tr.drainCount)
	}
	if len(tr.writes) != 3 {
		t.Fatalf("writes = %d, want 3", len(tr.writes))
	}
	if tr.writes[0][2] != hyperxSubOnline {
		t.Errorf("write[0] subcmd = 0x%02X, want 0x%02X", tr.writes[0][2], hyperxSubOnline)
	}
	if tr.writes[1][2] != hyperxSubCharge {
		t.Errorf("write[1] subcmd = 0x%02X", tr.writes[1][2])
	}
	if tr.writes[2][2] != hyperxSubBattery {
		t.Errorf("write[2] subcmd = 0x%02X", tr.writes[2][2])
	}

	if cbFired != 1 {
		t.Errorf("callback fired %d times, want 1", cbFired)
	}
	if !cbState.IsConnected {
		t.Error("IsConnected = false, want true")
	}
	if cbState.Battery == nil || *cbState.Battery != 0x55 {
		t.Errorf("Battery = %v, want 85", cbState.Battery)
	}
	if cbState.IsCharging == nil || *cbState.IsCharging {
		t.Errorf("IsCharging = %v, want &false", cbState.IsCharging)
	}
}

func TestHyperX_PollOnce_HeadsetOff(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{
			{data: makeResp(hyperxSubOnline, 0x01)},
		},
	}
	d := newHarness(tr)

	var cbState protocol.DeviceState
	d.SetOnStateChange(func(s protocol.DeviceState) { cbState = s })

	d.pollOnce()

	if len(tr.writes) != 1 {
		t.Errorf("writes = %d, want 1 (should short-circuit)", len(tr.writes))
	}
	if cbState.IsConnected {
		t.Error("IsConnected should be false when headset is off")
	}
	if cbState.Battery != nil {
		t.Errorf("Battery = %v, want nil", cbState.Battery)
	}
}

func TestHyperX_PollOnce_Charging(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{
			{data: makeResp(hyperxSubOnline, 0x00)},
			{data: makeResp(hyperxSubCharge, 0x01)},
		},
	}
	d := newHarness(tr)

	var cbState protocol.DeviceState
	d.SetOnStateChange(func(s protocol.DeviceState) { cbState = s })

	d.pollOnce()

	if len(tr.writes) != 2 {
		t.Errorf("writes = %d, want 2 (should short-circuit before battery round)", len(tr.writes))
	}
	if !cbState.IsConnected {
		t.Error("IsConnected should be true when charging")
	}
	if cbState.Battery != nil {
		t.Errorf("Battery = %v, want nil when charging", cbState.Battery)
	}
	if cbState.IsCharging == nil || !*cbState.IsCharging {
		t.Errorf("IsCharging = %v, want &true", cbState.IsCharging)
	}
}

func TestHyperX_PollOnce_HeaderMismatchRetries(t *testing.T) {
	// First response to round 1 has a bogus header (simulates an unsolicited
	// input report arriving before our answer). Second response is the real
	// one. doRound should discard the first and accept the second.
	bogus := make([]byte, hyperxReportSize)
	bogus[0] = 0xFF
	bogus[1] = 0x00

	tr := &scriptedTransport{
		reads: []readResult{
			{data: bogus},
			{data: makeResp(hyperxSubOnline, 0x00)},
			{data: makeResp(hyperxSubCharge, 0x00)},
			{data: makeResp(hyperxSubBattery, 0x2A)},
		},
	}
	d := newHarness(tr)

	var cbState protocol.DeviceState
	d.SetOnStateChange(func(s protocol.DeviceState) { cbState = s })

	d.pollOnce()

	if cbState.Battery == nil || *cbState.Battery != 0x2A {
		t.Errorf("Battery = %v, want 42", cbState.Battery)
	}
}

func TestHyperX_PollOnce_TimeoutMarksDisconnected(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{
			{err: errHyperXReadTimeout},
		},
	}
	d := newHarness(tr)

	b := 99
	f := false
	d.state = protocol.DeviceState{
		DeviceID:    hyperxDeviceID,
		DeviceType:  string(DeviceTypeHyperXCloudAlpha),
		IsConnected: true,
		Battery:     &b,
		IsCharging:  &f,
	}

	var cbState protocol.DeviceState
	var cbFired int
	d.SetOnStateChange(func(s protocol.DeviceState) {
		cbFired++
		cbState = s
	})

	d.pollOnce()

	if cbFired != 1 {
		t.Errorf("callback fired %d times, want 1 (state changed)", cbFired)
	}
	if cbState.IsConnected {
		t.Error("IsConnected should be false after timeout")
	}
}

func TestHyperX_CallbackOnlyOnChange(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{
			{data: makeResp(hyperxSubOnline, 0x00)},
			{data: makeResp(hyperxSubCharge, 0x00)},
			{data: makeResp(hyperxSubBattery, 0x50)},
			{data: makeResp(hyperxSubOnline, 0x00)},
			{data: makeResp(hyperxSubCharge, 0x00)},
			{data: makeResp(hyperxSubBattery, 0x50)},
		},
	}
	d := newHarness(tr)

	var cbFired int
	d.SetOnStateChange(func(protocol.DeviceState) { cbFired++ })

	d.pollOnce()
	d.pollOnce()

	if cbFired != 1 {
		t.Errorf("callback fired %d times, want 1 (second poll had identical state)", cbFired)
	}
}

func TestHyperX_DoRound_GivesUpAfterRetries(t *testing.T) {
	bogus := make([]byte, hyperxReportSize)
	bogus[0] = 0xDE
	bogus[1] = 0xAD

	reads := make([]readResult, hyperxHeaderRetries)
	for i := range reads {
		reads[i] = readResult{data: bogus}
	}
	tr := &scriptedTransport{reads: reads}
	d := newHarness(tr)

	_, err := d.doRound(tr, hyperxSubOnline)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
}

func TestHyperX_DoRound_PropagatesTransportError(t *testing.T) {
	tr := &scriptedTransport{
		reads: []readResult{{err: errors.New("boom")}},
	}
	d := newHarness(tr)

	_, err := d.doRound(tr, hyperxSubOnline)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHyperX_BatteryDeviceInterface(t *testing.T) {
	var _ BatteryDevice = (*HyperXDevice)(nil)
}

func TestHyperX_GetIDNameType(t *testing.T) {
	d := NewHyperXDevice()
	if d.GetID() != hyperxDeviceID {
		t.Errorf("GetID = %q", d.GetID())
	}
	if d.GetName() != hyperxDeviceName {
		t.Errorf("GetName = %q", d.GetName())
	}
	if d.GetType() != DeviceTypeHyperXCloudAlpha {
		t.Errorf("GetType = %q", d.GetType())
	}
}
