package ble

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/bfarka/opendraft/internal/domain"
)

var (
	heartRateServiceUUID         = bluetooth.New16BitUUID(0x180D)
	heartRateMeasurementCharUUID = bluetooth.New16BitUUID(0x2A37)
)

// Source implements domain.HeartRateSource using tinygo.org/x/bluetooth.
type Source struct {
	adapter     *bluetooth.Adapter
	connectedFn func(context.Context) []domain.HRDevice

	mu        sync.Mutex
	enabled   bool
	scanning  bool
	scanDone  chan struct{}
	addresses map[string]bluetooth.Address
	connected *bluetooth.Device
}

// NewSource creates a BLE HeartRateSource backed by the default Bluetooth adapter.
func NewSource() *Source {
	return &Source{
		adapter:     bluetooth.DefaultAdapter,
		connectedFn: ConnectedHRDevices,
		addresses:   make(map[string]bluetooth.Address),
	}
}

func (s *Source) ensureEnabled() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return nil
	}
	if err := s.adapter.Enable(); err != nil {
		return fmt.Errorf("enable bluetooth adapter: %w", err)
	}
	s.enabled = true
	return nil
}

func (s *Source) stopScan() {
	s.mu.Lock()
	wasScanning := s.scanning
	done := s.scanDone
	s.scanning = false
	s.mu.Unlock()
	if wasScanning {
		_ = s.adapter.StopScan()
	}
	if done != nil {
		<-done
	}
}

// Scan discovers nearby BLE devices advertising the Heart Rate Service (0x180D)
// or matching common HR strap names (e.g. TICKR) until ctx is canceled.
func (s *Source) Scan(ctx context.Context) (<-chan domain.HRDevice, error) {
	if err := s.ensureEnabled(); err != nil {
		return nil, err
	}

	s.stopScan()

	done := make(chan struct{})
	s.mu.Lock()
	s.scanning = true
	s.scanDone = done
	s.mu.Unlock()

	out := make(chan domain.HRDevice, 16)
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	go func() {
		defer close(done)
		defer close(out)
		defer func() {
			s.mu.Lock()
			s.scanning = false
			s.mu.Unlock()
		}()
		go func() {
			<-ctx.Done()
			s.stopScan()
		}()

		if !s.emitConnected(ctx, out, seen, &seenMu) {
			return
		}

		_ = s.adapter.Scan(func(_ *bluetooth.Adapter, res bluetooth.ScanResult) {
			if ctx.Err() != nil {
				return
			}
			name := res.LocalName()
			if !res.HasServiceUUID(heartRateServiceUUID) && !looksLikeHRMonitor(name) {
				return
			}
			id := res.Address.String()
			s.mu.Lock()
			s.addresses[id] = res.Address
			s.mu.Unlock()

			seenMu.Lock()
			_, already := seen[id]
			if !already {
				seen[id] = struct{}{}
			}
			seenMu.Unlock()
			if already {
				return
			}

			if name == "" {
				name = id
			}
			select {
			case out <- domain.HRDevice{ID: id, Name: name, RSSI: res.RSSI}:
			case <-ctx.Done():
			}
		})
	}()

	return out, nil
}

// Connect connects to the given BLE device ID, subscribes to 0x2A37 notifications,
// and streams decoded HR readings until ctx is canceled or Close is called.
func (s *Source) Connect(ctx context.Context, deviceID string) (<-chan domain.HRReading, error) {
	if err := s.ensureEnabled(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	addr, ok := s.addresses[deviceID]
	s.mu.Unlock()
	if !ok {
		addr.Set(deviceID)
		if addr == (bluetooth.Address{}) {
			return nil, fmt.Errorf("ble device %q not found in recent scan", deviceID)
		}
	}

	// Stop any active scan once before initiating a BLE connection on macOS/Linux.
	s.stopScan()

	dev, err := s.adapter.Connect(addr, bluetooth.ConnectionParams{
		ConnectionTimeout: bluetooth.NewDuration(10 * time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", deviceID, err)
	}
	if dev.Address == (bluetooth.Address{}) {
		return nil, fmt.Errorf("connect to %s: peripheral disconnected during connect", deviceID)
	}

	svcs, err := dev.DiscoverServices(nil)
	if err != nil {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("discover services: %w", err)
	}
	var hrSvc *bluetooth.DeviceService
	for i := range svcs {
		if svcs[i].UUID() == heartRateServiceUUID {
			hrSvc = &svcs[i]
			break
		}
	}
	if hrSvc == nil {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("heart rate service 0x180D not found on %s", deviceID)
	}

	chars, err := hrSvc.DiscoverCharacteristics(nil)
	if err != nil {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("discover characteristics: %w", err)
	}
	var hrChar *bluetooth.DeviceCharacteristic
	for i := range chars {
		if chars[i].UUID() == heartRateMeasurementCharUUID {
			hrChar = &chars[i]
			break
		}
	}
	if hrChar == nil {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("heart rate characteristic 0x2A37 not found on %s", deviceID)
	}

	s.mu.Lock()
	if s.connected != nil {
		_ = s.connected.Disconnect()
	}
	s.connected = &dev
	s.mu.Unlock()

	stream := newHRStream(16)
	if err := hrChar.EnableNotifications(func(buf []byte) {
		bpm, parseErr := ParseHeartRateMeasurement(buf)
		if parseErr != nil {
			return
		}
		stream.send(domain.HRReading{BPM: bpm, Timestamp: time.Now()})
	}); err != nil {
		_ = dev.Disconnect()
		stream.close()
		return nil, fmt.Errorf("enable HR notifications: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = hrChar.EnableNotifications(nil)
		_ = dev.Disconnect()
		stream.close()
	}()

	return stream.ch, nil
}

type hrStream struct {
	ch     chan domain.HRReading
	mu     sync.Mutex
	closed bool
}

func newHRStream(bufSize int) *hrStream {
	return &hrStream{ch: make(chan domain.HRReading, bufSize)}
}

func (s *hrStream) send(r domain.HRReading) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- r:
	default:
	}
}

func (s *hrStream) close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
	s.mu.Unlock()
}

// Close disconnects any active BLE peripheral.
func (s *Source) Close() error {
	s.mu.Lock()
	dev := s.connected
	s.connected = nil
	s.mu.Unlock()
	if dev != nil {
		return dev.Disconnect()
	}
	return nil
}

func (s *Source) emitConnected(ctx context.Context, out chan<- domain.HRDevice, seen map[string]struct{}, seenMu *sync.Mutex) bool {
	if s.connectedFn == nil {
		return true
	}
	for _, dev := range s.connectedFn(ctx) {
		if ctx.Err() != nil {
			return false
		}
		var addr bluetooth.Address
		addr.Set(dev.ID)
		s.mu.Lock()
		s.addresses[dev.ID] = addr
		s.mu.Unlock()

		seenMu.Lock()
		_, already := seen[dev.ID]
		if !already {
			seen[dev.ID] = struct{}{}
		}
		seenMu.Unlock()
		if already {
			continue
		}

		select {
		case out <- dev:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func looksLikeHRMonitor(name string) bool {
	u := strings.ToUpper(name)
	return strings.Contains(u, "TICKR") || strings.Contains(u, "HRM") || strings.Contains(u, "HEART")
}
