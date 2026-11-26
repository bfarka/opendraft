package multi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bfarka/opendraft/internal/domain"
)

type stubSource struct {
	devices []domain.HRDevice
	scanErr error
	bpm     uint8
	lastID  string
	closed  bool
}

func (s *stubSource) Scan(ctx context.Context) (<-chan domain.HRDevice, error) {
	if s.scanErr != nil {
		return nil, s.scanErr
	}
	ch := make(chan domain.HRDevice, len(s.devices))
	for _, d := range s.devices {
		ch <- d
	}
	close(ch)
	return ch, nil
}

func (s *stubSource) Connect(_ context.Context, deviceID string) (<-chan domain.HRReading, error) {
	s.lastID = deviceID
	ch := make(chan domain.HRReading, 1)
	ch <- domain.HRReading{BPM: s.bpm, Timestamp: time.Now()}
	close(ch)
	return ch, nil
}

func (s *stubSource) Close() error {
	s.closed = true
	return nil
}

func TestMultiScanMergesAndDegradesGracefully(t *testing.T) {
	ble := &stubSource{
		devices: []domain.HRDevice{{ID: "AA:BB:CC", Name: "TICKR", RSSI: -60}},
	}
	ant := &stubSource{
		devices: []domain.HRDevice{{ID: "12345", Name: "ANT+ HR 12345", RSSI: -55, Transport: "ant"}},
	}

	t.Run("both sources succeed", func(t *testing.T) {
		m := New(NamedSource{Name: "ble", Source: ble}, NamedSource{Name: "ant", Source: ant})
		ch, err := m.Scan(context.Background())
		if err != nil {
			t.Fatalf("Scan() error: %v", err)
		}
		got := make(map[string]domain.HRDevice)
		for d := range ch {
			got[d.ID] = d
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 devices, got %d (%+v)", len(got), got)
		}
		if got["ble:AA:BB:CC"].Transport != "ble" {
			t.Fatalf("expected ble transport, got %+v", got["ble:AA:BB:CC"])
		}
		if got["ant:12345"].Transport != "ant" {
			t.Fatalf("expected ant transport, got %+v", got["ant:12345"])
		}
	})

	t.Run("one source fails gracefully", func(t *testing.T) {
		failingANT := &stubSource{scanErr: errors.New("no stick")}
		m := New(NamedSource{Name: "ble", Source: ble}, NamedSource{Name: "ant", Source: failingANT})
		ch, err := m.Scan(context.Background())
		if err != nil {
			t.Fatalf("expected graceful fallback, got error: %v", err)
		}
		var count int
		for range ch {
			count++
		}
		if count != 1 {
			t.Fatalf("expected 1 device from surviving source, got %d", count)
		}
	})

	t.Run("all sources fail returns error", func(t *testing.T) {
		m := New(
			NamedSource{Name: "ble", Source: &stubSource{scanErr: errors.New("ble down")}},
			NamedSource{Name: "ant", Source: &stubSource{scanErr: errors.New("no stick")}},
		)
		if _, err := m.Scan(context.Background()); err == nil {
			t.Fatal("expected error when all sources fail")
		}
	})
}

func TestMultiConnectRouting(t *testing.T) {
	ble := &stubSource{bpm: 130}
	ant := &stubSource{bpm: 145}
	m := New(NamedSource{Name: "ble", Source: ble}, NamedSource{Name: "ant", Source: ant})

	ctx := context.Background()

	ch, err := m.Connect(ctx, "ant:12345")
	if err != nil {
		t.Fatalf("Connect ant error: %v", err)
	}
	if r := <-ch; r.BPM != 145 || ant.lastID != "12345" {
		t.Fatalf("unexpected ant connect result: bpm=%d lastID=%q", r.BPM, ant.lastID)
	}

	ch, err = m.Connect(ctx, "ble:AA:BB:CC")
	if err != nil {
		t.Fatalf("Connect ble error: %v", err)
	}
	if r := <-ch; r.BPM != 130 || ble.lastID != "AA:BB:CC" {
		t.Fatalf("unexpected ble connect result: bpm=%d lastID=%q", r.BPM, ble.lastID)
	}

	// Legacy unprefixed MAC routes to BLE
	ch, err = m.Connect(ctx, "11:22:33:44:55:66")
	if err != nil {
		t.Fatalf("Connect legacy ble error: %v", err)
	}
	if r := <-ch; r.BPM != 130 || ble.lastID != "11:22:33:44:55:66" {
		t.Fatalf("unexpected legacy ble connect result: bpm=%d lastID=%q", r.BPM, ble.lastID)
	}
}

func TestMatchDeviceID(t *testing.T) {
	tests := []struct {
		saved      string
		discovered string
		want       bool
	}{
		{"ant:12345", "ant:12345", true},
		{"ble:AA:BB:CC", "ble:AA:BB:CC", true},
		{"AA:BB:CC:DD:EE:FF", "ble:AA:BB:CC:DD:EE:FF", true},
		{"hr-tickr-1", "ble:hr-tickr-1", true},
		{"ant:12345", "ble:12345", false},
		{"", "ant:12345", false},
	}
	for _, tc := range tests {
		if got := MatchDeviceID(tc.saved, tc.discovered); got != tc.want {
			t.Errorf("MatchDeviceID(%q, %q) = %v, want %v", tc.saved, tc.discovered, got, tc.want)
		}
	}
}
