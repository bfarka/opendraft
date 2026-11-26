package ant

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type mockTransport struct {
	mu      sync.Mutex
	written bytes.Buffer
	r       *io.PipeReader
	w       *io.PipeWriter
	closed  bool
}

func newMockTransport() *mockTransport {
	r, w := io.Pipe()
	return &mockTransport{r: r, w: w}
}

func (m *mockTransport) Read(p []byte) (int, error) {
	return m.r.Read(p)
}

func (m *mockTransport) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.written.Write(p)
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()
	_ = m.w.Close()
	return m.r.Close()
}

func (m *mockTransport) pushFrame(f []byte) {
	_, _ = m.w.Write(f)
}

func TestSourceScan(t *testing.T) {
	tr := newMockTransport()
	src := NewSourceWithOpener(func(_ context.Context) (Transport, error) {
		return tr, nil
	})
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := src.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	// Send an extended HRM broadcast from device 12345 and a non-HRM device 9999.
	nonHRM := EncodeFrame(msgBroadcastData,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 90,
		0x80, 0x0F, 0x27, 0x0B, 0x01, // DeviceType 11 (power), not 120 (HRM)
	)
	hrm := EncodeFrame(msgBroadcastData,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 144,
		0xC0, 0x39, 0x30, 0x78, 0x01, 0x10, 0xC6, 0x00, // Dev 12345, HRM (0x78), RSSI -58
	)
	tr.pushFrame(nonHRM)
	tr.pushFrame(hrm)
	tr.pushFrame(hrm) // duplicate should be deduplicated

	select {
	case dev := <-ch:
		if dev.ID != "12345" || dev.Transport != "ant" || dev.RSSI != -58 {
			t.Fatalf("unexpected HRDevice: %+v", dev)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for scanned ANT+ HR device")
	}

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected scan channel to close after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("scan channel did not close")
	}
}

func TestSourceConnect(t *testing.T) {
	tr := newMockTransport()
	src := NewSourceWithOpener(func(_ context.Context) (Transport, error) {
		return tr, nil
	})
	defer src.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := src.Connect(ctx, "not-a-number"); err == nil {
		t.Fatal("expected error for invalid ANT+ device ID")
	}

	ch, err := src.Connect(ctx, "12345")
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	// BPM 0 should be ignored, BPM 152 should be emitted.
	tr.pushFrame(EncodeFrame(msgBroadcastData, 0x00, 0, 0, 0, 0, 0, 0, 0, 0))
	tr.pushFrame(EncodeFrame(msgBroadcastData, 0x00, 0, 0, 0, 0, 0, 0, 0, 152))

	select {
	case reading := <-ch:
		if reading.BPM != 152 {
			t.Fatalf("expected BPM 152, got %d", reading.BPM)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for HRReading")
	}
}
