//go:build linux

package ant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	usbSysfsDir = "/sys/bus/usb/devices"
	usbDevfsDir = "/dev/bus/usb"

	// Known ANT+ USB stick vendor/product IDs (Dynastream / Garmin / compatible clones).
	antVendorID = 0x0fcf
)

var antProductIDs = map[uint16]bool{
	0x1008: true, // ANTUSB2 Stick
	0x1009: true, // ANTUSB-m Stick
	0x1004: true, // ANTUSB1 Stick (older)
}

// Linux <linux/usbdevice_fs.h> ioctl definitions.
const (
	usbdevfsClaimInterface   uintptr = 0x8004550f // _IOR('U', 15, unsigned int)
	usbdevfsReleaseInterface uintptr = 0x80045510 // _IOR('U', 16, unsigned int)
)

type usbdevfsBulkTransfer struct {
	ep      uint32
	length  uint32
	timeout uint32
	data    unsafe.Pointer
}

var usbdevfsBulkIoctl = uintptr(0xC0000000) | (unsafe.Sizeof(usbdevfsBulkTransfer{}) << 16) | (uintptr('U') << 8) | 2

type usbStickInfo struct {
	devPath string
	epIn    uint32
	epOut   uint32
}

// usbfsTransport implements Transport over Linux /dev/bus/usb using USBDEVFS_BULK ioctls.
type usbfsTransport struct {
	file   *os.File
	iface  uint32
	epIn   uint32
	epOut  uint32
	mu     sync.Mutex
	closed bool
}

// OpenUSBStick finds and opens the first available ANT+ USB stick on Linux.
func OpenUSBStick(_ context.Context) (Transport, error) {
	info, err := findANTStick()
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(info.devPath, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) {
			return nil, fmt.Errorf("ant: permission denied opening %s (add udev rule: SUBSYSTEM==\"usb\", ATTR{idVendor}==\"0fcf\", MODE=\"0666\"): %w", info.devPath, err)
		}
		return nil, fmt.Errorf("ant: open %s: %w", info.devPath, err)
	}

	iface := uint32(0)
	if err := ioctlPtr(f.Fd(), usbdevfsClaimInterface, unsafe.Pointer(&iface)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("ant: claim USB interface on %s: %w", info.devPath, err)
	}

	return &usbfsTransport{
		file:  f,
		iface: iface,
		epIn:  info.epIn,
		epOut: info.epOut,
	}, nil
}

func (t *usbfsTransport) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		t.mu.Lock()
		closed := t.closed
		fd := t.file.Fd()
		t.mu.Unlock()
		if closed {
			return 0, os.ErrClosed
		}

		xfer := usbdevfsBulkTransfer{
			ep:      t.epIn,
			length:  uint32(len(p)),
			timeout: 250, // ms, allows prompt cancellation check
			data:    unsafe.Pointer(&p[0]),
		}
		n, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, usbdevfsBulkIoctl, uintptr(unsafe.Pointer(&xfer)))
		if errno == 0 {
			return int(n), nil
		}
		if errno == syscall.ETIMEDOUT || errno == syscall.EINTR {
			continue
		}
		return 0, fmt.Errorf("ant: usb bulk read: %w", errno)
	}
}

func (t *usbfsTransport) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	t.mu.Lock()
	closed := t.closed
	fd := t.file.Fd()
	t.mu.Unlock()
	if closed {
		return 0, os.ErrClosed
	}

	xfer := usbdevfsBulkTransfer{
		ep:      t.epOut,
		length:  uint32(len(p)),
		timeout: 1000,
		data:    unsafe.Pointer(&p[0]),
	}
	n, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, usbdevfsBulkIoctl, uintptr(unsafe.Pointer(&xfer)))
	if errno != 0 {
		return 0, fmt.Errorf("ant: usb bulk write: %w", errno)
	}
	return int(n), nil
}

func (t *usbfsTransport) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	iface := t.iface
	_ = ioctlPtr(t.file.Fd(), usbdevfsReleaseInterface, unsafe.Pointer(&iface))
	return t.file.Close()
}

func ioctlPtr(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func findANTStick() (usbStickInfo, error) {
	entries, err := os.ReadDir(usbSysfsDir)
	if err != nil {
		return usbStickInfo{}, fmt.Errorf("%w: %v", ErrNoStick, err)
	}

	for _, entry := range entries {
		dir := filepath.Join(usbSysfsDir, entry.Name())
		vid, ok := readHexUint16(filepath.Join(dir, "idVendor"))
		if !ok || vid != antVendorID {
			continue
		}
		pid, ok := readHexUint16(filepath.Join(dir, "idProduct"))
		if !ok || !antProductIDs[pid] {
			continue
		}
		busnum, ok1 := readDecInt(filepath.Join(dir, "busnum"))
		devnum, ok2 := readDecInt(filepath.Join(dir, "devnum"))
		if !ok1 || !ok2 {
			continue
		}
		epIn, epOut := discoverEndpoints(dir)
		return usbStickInfo{
			devPath: fmt.Sprintf("%s/%03d/%03d", usbDevfsDir, busnum, devnum),
			epIn:    epIn,
			epOut:   epOut,
		}, nil
	}
	return usbStickInfo{}, ErrNoStick
}

func discoverEndpoints(devDir string) (epIn, epOut uint32) {
	epIn = 0x81
	epOut = 0x01
	matches, err := filepath.Glob(filepath.Join(devDir, "*:1.0", "ep_*", "bEndpointAddress"))
	if err != nil {
		return epIn, epOut
	}
	for _, m := range matches {
		addr, ok := readHexUint16(m)
		if !ok {
			continue
		}
		epDir := filepath.Dir(m)
		typBytes, err := os.ReadFile(filepath.Join(epDir, "type"))
		if err != nil || strings.TrimSpace(string(typBytes)) != "Bulk" {
			continue
		}
		if addr&0x80 != 0 {
			epIn = uint32(addr)
		} else {
			epOut = uint32(addr)
		}
	}
	return epIn, epOut
}

func readHexUint16(path string) (uint16, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	s = strings.TrimPrefix(s, "0x")
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}

func readDecInt(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return v, true
}
