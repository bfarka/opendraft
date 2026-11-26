package ant

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/bfarka/opendraft/internal/domain"
)

// Source implements domain.HeartRateSource for ANT+ USB sticks.
type Source struct {
	open       Opener
	resetDelay time.Duration

	mu     sync.Mutex
	active Transport
}

// NewSource creates an ANT+ HeartRateSource backed by the default Linux usbfs opener.
func NewSource() *Source {
	return &Source{
		open:       OpenUSBStick,
		resetDelay: 250 * time.Millisecond,
	}
}

// NewSourceWithOpener creates an ANT+ HeartRateSource with a custom Transport opener (useful for testing).
func NewSourceWithOpener(open Opener) *Source {
	return &Source{
		open:       open,
		resetDelay: 10 * time.Millisecond,
	}
}

func (s *Source) setActive(tr Transport) {
	s.mu.Lock()
	old := s.active
	s.active = tr
	s.mu.Unlock()
	if old != nil && old != tr {
		_ = old.Close()
	}
}

// Close releases any active ANT+ USB transport.
func (s *Source) Close() error {
	s.mu.Lock()
	tr := s.active
	s.active = nil
	s.mu.Unlock()
	if tr != nil {
		return tr.Close()
	}
	return nil
}

// Scan opens the ANT+ USB stick in continuous Rx scan mode and emits discovered ANT+ HR monitors.
func (s *Source) Scan(ctx context.Context) (<-chan domain.HRDevice, error) {
	tr, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	s.setActive(tr)

	if err := s.configureHRMChannel(ctx, tr, 0, true); err != nil {
		_ = tr.Close()
		s.setActive(nil)
		return nil, err
	}

	out := make(chan domain.HRDevice, 16)
	go func() {
		defer close(out)
		defer func() {
			_, _ = tr.Write(cmdCloseChannel(0))
			_ = tr.Close()
			s.mu.Lock()
			if s.active == tr {
				s.active = nil
			}
			s.mu.Unlock()
		}()

		go func() {
			<-ctx.Done()
			_, _ = tr.Write(cmdCloseChannel(0))
			_ = tr.Close()
		}()

		seen := make(map[uint16]struct{})
		readFrames(ctx, tr, func(f Frame) {
			if f.MsgID != msgBroadcastData && f.MsgID != msgAcknowledgedData {
				return
			}
			b, err := ParseHRBroadcast(f.Payload)
			if err != nil || !b.HasExtID || b.DeviceNumber == 0 {
				return
			}
			if b.DeviceType != 0 && b.DeviceType != hrmDeviceType {
				return
			}
			if _, ok := seen[b.DeviceNumber]; ok {
				return
			}
			seen[b.DeviceNumber] = struct{}{}

			rssi := int16(0)
			if b.HasRSSI {
				rssi = b.RSSI
			}
			dev := domain.HRDevice{
				ID:        strconv.Itoa(int(b.DeviceNumber)),
				Name:      fmt.Sprintf("ANT+ HR %d", b.DeviceNumber),
				RSSI:      rssi,
				Transport: "ant",
			}
			select {
			case out <- dev:
			case <-ctx.Done():
			}
		})
	}()

	return out, nil
}

// Connect pairs with a specific ANT+ HR monitor (16-bit device number) and streams live BPM readings.
func (s *Source) Connect(ctx context.Context, deviceID string) (<-chan domain.HRReading, error) {
	num, err := strconv.ParseUint(deviceID, 10, 16)
	if err != nil || num == 0 {
		return nil, fmt.Errorf("ant: invalid HR device ID %q: must be 1..65535", deviceID)
	}

	tr, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	s.setActive(tr)

	if err := s.configureHRMChannel(ctx, tr, uint16(num), false); err != nil {
		_ = tr.Close()
		s.setActive(nil)
		return nil, err
	}

	out := make(chan domain.HRReading, 32)
	go func() {
		defer close(out)
		defer func() {
			_, _ = tr.Write(cmdCloseChannel(0))
			_ = tr.Close()
			s.mu.Lock()
			if s.active == tr {
				s.active = nil
			}
			s.mu.Unlock()
		}()

		go func() {
			<-ctx.Done()
			_, _ = tr.Write(cmdCloseChannel(0))
			_ = tr.Close()
		}()

		readFrames(ctx, tr, func(f Frame) {
			if f.MsgID != msgBroadcastData && f.MsgID != msgAcknowledgedData {
				return
			}
			b, err := ParseHRBroadcast(f.Payload)
			if err != nil || b.BPM == 0 {
				return
			}
			select {
			case out <- domain.HRReading{BPM: b.BPM, Timestamp: time.Now()}:
			case <-ctx.Done():
			}
		})
	}()

	return out, nil
}

func (s *Source) configureHRMChannel(ctx context.Context, tr Transport, deviceNum uint16, rxScan bool) error {
	if _, err := tr.Write(cmdResetSystem()); err != nil {
		return fmt.Errorf("ant: reset system: %w", err)
	}
	if s.resetDelay > 0 {
		select {
		case <-time.After(s.resetDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	cmds := [][]byte{
		cmdSetNetworkKey(0),
		cmdAssignChannel(0, 0x00, 0),
		cmdSetChannelID(0, deviceNum, hrmDeviceType, 0),
		cmdSetChannelPeriod(0, hrmChannelPeriod),
		cmdSetChannelRFFreq(0, antPlusRFFreq),
		cmdSetChannelTimeout(0, 0xFF),
	}
	if rxScan {
		cmds = append(cmds, cmdLibConfig(libConfigAll), cmdOpenRxScanMode())
	} else {
		cmds = append(cmds, cmdOpenChannel(0))
	}

	for _, cmd := range cmds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := tr.Write(cmd); err != nil {
			return fmt.Errorf("ant: write config command: %w", err)
		}
	}
	return nil
}

func readFrames(ctx context.Context, r io.Reader, onFrame func(Frame)) {
	var dec FrameDecoder
	buf := make([]byte, 256)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := r.Read(buf)
		if n > 0 {
			for _, f := range dec.Feed(buf[:n]) {
				onFrame(f)
			}
		}
		if err != nil {
			return
		}
	}
}
