package device

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jyablonski/goarctis/pkg/protocol"
	"golang.org/x/sys/unix"
)

const (
	hyperxVendorID  = 0x03F0
	hyperxProductID = 0x098D

	hyperxReportSize   = 31
	hyperxReadTimeout  = 2 * time.Second
	hyperxPollInterval = 5 * time.Second

	hyperxCmdHeader0 = 0x21
	hyperxCmdHeader1 = 0xBB
	hyperxSubOnline  = 0x03
	hyperxSubCharge  = 0x0C
	hyperxSubBattery = 0x0B

	// Max times we re-read after a header mismatch (unsolicited input reports,
	// stale responses). Small bound — after this we treat the round as failed
	// and the next tick's drain will clean up.
	hyperxHeaderRetries = 3

	hyperxDeviceID   = "hyperx_cloud_alpha_wireless"
	hyperxDeviceName = "HyperX Cloud Alpha Wireless"
)

var errHyperXReadTimeout = errors.New("hyperx read timeout")

// hidTransport abstracts raw HID read/write so the three-round protocol,
// drain, and header validation are testable without real syscalls.
type hidTransport interface {
	Write(p []byte) (int, error)
	ReadTimeout(p []byte, timeout time.Duration) (int, error)
	Drain() error
	Close() error
}

// realHIDTransport wraps an O_RDWR|O_NONBLOCK hidraw file descriptor.
// We use raw syscalls (not *os.File) because Go's os package can layer in
// its runtime poller in ways that interact surprisingly with non-blocking
// character devices.
type realHIDTransport struct {
	fd int
}

func openRealHIDTransport(path string) (hidTransport, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &realHIDTransport{fd: fd}, nil
}

func (t *realHIDTransport) Write(p []byte) (int, error) {
	return unix.Write(t.fd, p)
}

func (t *realHIDTransport) ReadTimeout(p []byte, timeout time.Duration) (int, error) {
	fds := []unix.PollFd{{Fd: int32(t.fd), Events: unix.POLLIN}}
	ms := int(timeout / time.Millisecond)
	for {
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("poll: %w", err)
		}
		if n == 0 {
			return 0, errHyperXReadTimeout
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			return 0, fmt.Errorf("poll revents=0x%x", fds[0].Revents)
		}
		return unix.Read(t.fd, p)
	}
}

func (t *realHIDTransport) Drain() error {
	buf := make([]byte, hyperxReportSize)
	for {
		n, err := unix.Read(t.fd, buf)
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return nil
		}
		if err != nil {
			return fmt.Errorf("drain: %w", err)
		}
		log.Printf("🎧 HyperX: drained stale report (%d bytes): %s", n, hex.EncodeToString(buf[:n]))
	}
}

func (t *realHIDTransport) Close() error {
	return unix.Close(t.fd)
}

type HyperXDevice struct {
	fs           FileSystem
	hidrawPath   string
	interfaceNum string

	transportNew func(path string) (hidTransport, error)
	transport    hidTransport

	state    protocol.DeviceState
	mu       sync.RWMutex
	onChange func(protocol.DeviceState)

	stopChan chan struct{}
	stopped  bool
}

func NewHyperXDevice() *HyperXDevice {
	return &HyperXDevice{
		fs:           RealFileSystem{},
		transportNew: openRealHIDTransport,
		state: protocol.DeviceState{
			DeviceID:    hyperxDeviceID,
			DeviceType:  string(DeviceTypeHyperXCloudAlpha),
			IsConnected: false,
		},
		stopChan: make(chan struct{}),
	}
}

func NewHyperXDeviceWithDeps(fs FileSystem, transportNew func(path string) (hidTransport, error)) *HyperXDevice {
	return &HyperXDevice{
		fs:           fs,
		transportNew: transportNew,
		state: protocol.DeviceState{
			DeviceID:    hyperxDeviceID,
			DeviceType:  string(DeviceTypeHyperXCloudAlpha),
			IsConnected: false,
		},
		stopChan: make(chan struct{}),
	}
}

func (h *HyperXDevice) FindDevice() error {
	matches, err := findHIDRawDevices(h.fs, hyperxVendorID, hyperxProductID)
	if err != nil {
		return fmt.Errorf("read hidraw: %w", err)
	}

	for _, match := range matches {
		h.hidrawPath = match.Path
		h.interfaceNum = match.InterfaceNumber
		log.Printf("🎧 HyperX: found device at %s (interface %s)", h.hidrawPath, match.InterfaceNumber)
		return nil
	}

	return fmt.Errorf("HyperX Cloud Alpha Wireless not found")
}

func (h *HyperXDevice) GetID() string { return hyperxDeviceID }

func (h *HyperXDevice) GetName() string { return hyperxDeviceName }

func (h *HyperXDevice) GetType() DeviceType { return DeviceTypeHyperXCloudAlpha }

func (h *HyperXDevice) GetState() protocol.DeviceState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state
}

func (h *HyperXDevice) IsConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state.IsConnected
}

func (h *HyperXDevice) SetOnStateChange(callback func(protocol.DeviceState)) {
	h.mu.Lock()
	h.onChange = callback
	h.mu.Unlock()
}

func (h *HyperXDevice) Start() error {
	if h.hidrawPath == "" {
		return fmt.Errorf("device path not set; call FindDevice first")
	}

	tr, err := h.transportNew(h.hidrawPath)
	if err != nil {
		return fmt.Errorf("open hyperx transport: %w", err)
	}
	h.mu.Lock()
	h.transport = tr
	h.mu.Unlock()

	log.Printf("🎧 HyperX: starting poll loop at %v", hyperxPollInterval)
	go h.pollLoop()
	return nil
}

func (h *HyperXDevice) Stop() error {
	h.mu.Lock()
	if !h.stopped {
		h.stopped = true
		close(h.stopChan)
	}
	h.mu.Unlock()
	return nil
}

func (h *HyperXDevice) Close() error {
	h.Stop()
	h.mu.Lock()
	tr := h.transport
	h.transport = nil
	h.mu.Unlock()
	if tr != nil {
		return tr.Close()
	}
	return nil
}

func (h *HyperXDevice) pollLoop() {
	// Fire immediately so the tray populates without a 5-second delay on startup.
	h.pollOnce()

	ticker := time.NewTicker(hyperxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.pollOnce()
		}
	}
}

func (h *HyperXDevice) pollOnce() {
	h.mu.RLock()
	tr := h.transport
	h.mu.RUnlock()
	if tr == nil {
		return
	}

	if err := tr.Drain(); err != nil {
		log.Printf("🎧 HyperX: drain failed: %v", err)
		h.setDisconnected()
		return
	}

	resp, err := h.doRound(tr, hyperxSubOnline)
	if err != nil {
		log.Printf("🎧 HyperX: online check failed: %v", err)
		h.setDisconnected()
		return
	}
	if resp[3] == 0x01 {
		// Headset powered off / out of range. Dongle present, but we treat
		// this as "not connected" per the plan — hide the tray section.
		h.setDisconnected()
		return
	}

	resp, err = h.doRound(tr, hyperxSubCharge)
	if err != nil {
		log.Printf("🎧 HyperX: charge check failed: %v", err)
		h.setDisconnected()
		return
	}
	if resp[3] == 0x01 {
		h.setCharging()
		return
	}

	resp, err = h.doRound(tr, hyperxSubBattery)
	if err != nil {
		log.Printf("🎧 HyperX: battery read failed: %v", err)
		h.setDisconnected()
		return
	}
	h.setBattery(int(resp[3]))
}

// doRound writes a request and reads the matching response, retrying on
// header mismatch (unsolicited input reports that arrived between drain
// and read). Returns the first response whose header matches.
func (h *HyperXDevice) doRound(tr hidTransport, subcmd byte) ([]byte, error) {
	req := make([]byte, hyperxReportSize)
	req[0] = hyperxCmdHeader0
	req[1] = hyperxCmdHeader1
	req[2] = subcmd

	if _, err := tr.Write(req); err != nil {
		return nil, fmt.Errorf("write subcmd 0x%02X: %w", subcmd, err)
	}

	resp := make([]byte, hyperxReportSize)
	for i := 0; i < hyperxHeaderRetries; i++ {
		n, err := tr.ReadTimeout(resp, hyperxReadTimeout)
		if err != nil {
			return nil, fmt.Errorf("read subcmd 0x%02X: %w", subcmd, err)
		}
		if n < 4 {
			log.Printf("🎧 HyperX: short response (%d bytes) for 0x%02X, retrying", n, subcmd)
			continue
		}
		if resp[0] == hyperxCmdHeader0 && resp[1] == hyperxCmdHeader1 && resp[2] == subcmd {
			return resp, nil
		}
		log.Printf("🎧 HyperX: header mismatch for 0x%02X (got %02X %02X %02X), retrying",
			subcmd, resp[0], resp[1], resp[2])
	}
	return nil, fmt.Errorf("no valid response for 0x%02X after %d reads", subcmd, hyperxHeaderRetries)
}

func (h *HyperXDevice) setBattery(level int) {
	isCharging := false
	h.setState(protocol.DeviceState{
		DeviceID:    hyperxDeviceID,
		DeviceType:  string(DeviceTypeHyperXCloudAlpha),
		IsConnected: true,
		Battery:     &level,
		IsCharging:  &isCharging,
	})
	log.Printf("🎧 HyperX: battery %d%%", level)
}

func (h *HyperXDevice) setCharging() {
	isCharging := true
	h.setState(protocol.DeviceState{
		DeviceID:    hyperxDeviceID,
		DeviceType:  string(DeviceTypeHyperXCloudAlpha),
		IsConnected: true,
		IsCharging:  &isCharging,
	})
	log.Printf("🎧 HyperX: charging over USB-C")
}

func (h *HyperXDevice) setDisconnected() {
	h.setState(protocol.DeviceState{
		DeviceID:    hyperxDeviceID,
		DeviceType:  string(DeviceTypeHyperXCloudAlpha),
		IsConnected: false,
	})
}

func (h *HyperXDevice) setState(newState protocol.DeviceState) {
	h.mu.Lock()
	changed := !h.state.Equal(newState)
	h.state = newState
	cb := h.onChange
	h.mu.Unlock()
	if changed && cb != nil {
		cb(newState)
	}
}
