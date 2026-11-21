package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bfarka/opendraft/internal/config"
	"github.com/bfarka/opendraft/internal/control"
	"github.com/bfarka/opendraft/internal/domain"
	"github.com/bfarka/opendraft/internal/version"
)

type stubFan struct {
	mu     sync.Mutex
	dev    domain.FanDevice
	speeds []int
	offs   int
}

func (s *stubFan) SetSpeed(_ context.Context, percent int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speeds = append(s.speeds, percent)
	return nil
}

func (s *stubFan) Off(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offs++
	return nil
}

func (s *stubFan) Info() domain.FanDevice { return s.dev }

func newTestBridge(t *testing.T) (*Bridge, *control.Loop, *stubFan, string) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	mapper := control.LinearMapper{
		MinHR:    cfg.MinHR,
		MaxHR:    cfg.MaxHR,
		MinSpeed: cfg.MinSpeed,
		MaxSpeed: cfg.MaxSpeed,
	}
	fan := &stubFan{
		dev: domain.FanDevice{ID: "shelly-1", Name: "Fan Dimmer", IP: "192.168.1.50", Port: 80, Model: "shelly"},
	}
	loop := control.NewLoop(mapper, fan)
	bridge := NewBridge(context.Background(), loop, cfgPath, cfg)
	bridge.SetDevices(&domain.HRDevice{ID: "hr-1", Name: "TICKR"}, &fan.dev)
	return bridge, loop, fan, cfgPath
}

func TestWebUIServesIndexAndAPIState(t *testing.T) {
	bridge, loop, _, _ := newTestBridge(t)
	loop.HandleHR(context.Background(), domain.HRReading{BPM: 142, Timestamp: time.Now()})

	h := NewHandler(bridge)

	// Index page
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "opendraft") {
		t.Fatalf("GET / body missing opendraft title")
	}
	if !strings.Contains(rec.Body.String(), `id="app-version"`) {
		t.Fatalf("GET / body missing app-version element")
	}

	// API state
	req = httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state status = %d, want 200", rec.Code)
	}
	var st State
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if st.Version != version.String() {
		t.Fatalf("got Version=%q, want %q", st.Version, version.String())
	}
	if st.BPM != 142 || st.EffectiveBPM != 142 {
		t.Fatalf("got BPM=%d EffectiveBPM=%d, want 142/142", st.BPM, st.EffectiveBPM)
	}
	if len(st.History) == 0 || st.History[len(st.History)-1] != 142 {
		t.Fatalf("expected History ending with 142 as JSON int array, got %v", st.History)
	}
	if st.HRDevice == nil || st.HRDevice.Name != "TICKR" {
		t.Fatalf("unexpected HRDevice: %+v", st.HRDevice)
	}
	if st.FanDevice == nil || st.FanDevice.Host != "192.168.1.50" {
		t.Fatalf("unexpected FanDevice: %+v", st.FanDevice)
	}
}

func TestAPIControlsAndValidation(t *testing.T) {
	bridge, _, _, cfgPath := newTestBridge(t)
	h := NewHandler(bridge)

	doPost := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Start
	if rec := doPost("/api/start", ""); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/start status = %d", rec.Code)
	}
	if !bridge.Snapshot().Running {
		t.Fatalf("expected Running=true after start")
	}

	// Set manual speed
	if rec := doPost("/api/speed", `{"speed":65}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/speed status = %d: %s", rec.Code, rec.Body.String())
	}
	st := bridge.Snapshot()
	if st.Mode != "MANUAL" || st.TargetSpeed != 65 || st.AppliedSpeed != 65 {
		t.Fatalf("unexpected state after speed=65: %+v", st)
	}

	// Invalid speed
	if rec := doPost("/api/speed", `{"speed":150}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/speed 150 status = %d, want 400", rec.Code)
	}

	// Switch back to AUTO
	if rec := doPost("/api/mode", `{"mode":"auto"}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/mode auto status = %d", rec.Code)
	}
	if bridge.Snapshot().Mode != "AUTO" {
		t.Fatalf("expected Mode=AUTO")
	}

	// Invalid mode
	if rec := doPost("/api/mode", `{"mode":"turbo"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/mode turbo status = %d, want 400", rec.Code)
	}

	// Update curve & persist
	curveBody := `{"min_hr":115,"max_hr":170,"min_speed":20,"max_speed":95,"window_seconds":90,"percentile":75}`
	if rec := doPost("/api/curve", curveBody); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/curve status = %d: %s", rec.Code, rec.Body.String())
	}
	saved, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if saved.MinHR != 115 || saved.MaxHR != 170 || saved.MinSpeed != 20 || saved.MaxSpeed != 95 || saved.WindowSeconds != 90 || saved.Percentile != 75 {
		t.Fatalf("unexpected saved config: %+v", saved)
	}

	// Invalid curve
	if rec := doPost("/api/curve", `{"min_hr":180,"max_hr":160,"min_speed":10,"max_speed":90,"window_seconds":60,"percentile":80}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/curve invalid status = %d, want 400", rec.Code)
	}

	// Stop
	if rec := doPost("/api/stop", ""); rec.Code != http.StatusOK {
		t.Fatalf("POST /api/stop status = %d", rec.Code)
	}
	if bridge.Snapshot().Running {
		t.Fatalf("expected Running=false after stop")
	}
}

type pipeResponseWriter struct {
	header http.Header
	pw     *io.PipeWriter
}

func (p *pipeResponseWriter) Header() http.Header         { return p.header }
func (p *pipeResponseWriter) WriteHeader(int)             {}
func (p *pipeResponseWriter) Write(b []byte) (int, error) { return p.pw.Write(b) }
func (p *pipeResponseWriter) Flush()                      {}

func TestSSEStreamReceivesUpdates(t *testing.T) {
	bridge, loop, _, _ := newTestBridge(t)
	h := NewHandler(bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pr, pw := io.Pipe()
	defer pr.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	w := &pipeResponseWriter{header: make(http.Header), pw: pw}

	go func() {
		defer pw.Close()
		h.ServeHTTP(w, req)
	}()

	scanner := bufio.NewScanner(pr)
	readEvent := func() State {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				var st State
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &st); err != nil {
					t.Fatalf("unmarshal SSE: %v", err)
				}
				return st
			}
		}
		t.Fatalf("SSE stream ended unexpectedly: %v", scanner.Err())
		return State{}
	}

	first := readEvent()
	if first.BPM != 0 {
		t.Fatalf("expected initial BPM=0, got %d", first.BPM)
	}

	loop.HandleHR(context.Background(), domain.HRReading{BPM: 155, Timestamp: time.Now()})
	bridge.Notify()

	second := readEvent()
	if second.BPM != 155 {
		t.Fatalf("expected updated BPM=155, got %d", second.BPM)
	}
}

type stubHRSource struct {
	devices []domain.HRDevice
}

func (s *stubHRSource) Scan(_ context.Context) (<-chan domain.HRDevice, error) {
	ch := make(chan domain.HRDevice, len(s.devices))
	for _, d := range s.devices {
		ch <- d
	}
	close(ch)
	return ch, nil
}

func (s *stubHRSource) Connect(ctx context.Context, _ string) (<-chan domain.HRReading, error) {
	ch := make(chan domain.HRReading, 1)
	ch <- domain.HRReading{BPM: 148, Timestamp: time.Now()}
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (s *stubHRSource) Close() error { return nil }

type stubFanDiscoverer struct {
	devices []domain.FanDevice
}

func (s *stubFanDiscoverer) Scan(_ context.Context) (<-chan domain.FanDevice, error) {
	ch := make(chan domain.FanDevice, len(s.devices))
	for _, d := range s.devices {
		ch <- d
	}
	close(ch)
	return ch, nil
}

func TestWebUIDiscoveryAndSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	mapper := control.LinearMapper{
		MinHR:    cfg.MinHR,
		MaxHR:    cfg.MaxHR,
		MinSpeed: cfg.MinSpeed,
		MaxSpeed: cfg.MaxSpeed,
	}
	loop := control.NewLoop(mapper, nil)
	bridge := NewBridge(ctx, loop, cfgPath, cfg)
	h := NewHandler(bridge)

	// Without hardware configured, /api/scan fails cleanly.
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/scan without hardware = %d, want 400", rec.Code)
	}

	hrSrc := &stubHRSource{
		devices: []domain.HRDevice{{ID: "hr-tickr-1", Name: "TICKR FIT 1", RSSI: -54}},
	}
	fanDisc := &stubFanDiscoverer{
		devices: []domain.FanDevice{{ID: "shelly-dimmer-1", Name: "Shelly Dimmer 1", IP: "192.168.1.88", Port: 80, Model: "shelly"}},
	}
	var newFan *stubFan
	bridge.SetHardware(hrSrc, fanDisc, func(dev domain.FanDevice) domain.FanController {
		newFan = &stubFan{dev: dev}
		return newFan
	})

	var syncCalls int
	bridge.SetOnDeviceSync(func(_ *domain.HRDevice, _ *domain.FanDevice, _ config.Config, _ domain.FanController) {
		syncCalls++
	})

	// Trigger scan
	req = httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/scan status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// Wait briefly for scan goroutines to populate discovery results
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := bridge.Snapshot()
		if len(st.Discovery.HRDevices) == 1 && len(st.Discovery.FanDevices) == 1 && !st.Discovery.BLEScanning && !st.Discovery.FanScanning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := bridge.Snapshot()
	if len(st.Discovery.HRDevices) != 1 || st.Discovery.HRDevices[0].ID != "hr-tickr-1" {
		t.Fatalf("unexpected discovered HR devices: %+v", st.Discovery.HRDevices)
	}
	if len(st.Discovery.FanDevices) != 1 || st.Discovery.FanDevices[0].ID != "shelly-dimmer-1" {
		t.Fatalf("unexpected discovered fan devices: %+v", st.Discovery.FanDevices)
	}

	// Select fan dimmer
	req = httptest.NewRequest(http.MethodPost, "/api/devices/fan", bytes.NewBufferString(`{"id":"shelly-dimmer-1"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/devices/fan status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var fanState State
	if err := json.Unmarshal(rec.Body.Bytes(), &fanState); err != nil {
		t.Fatalf("decode fan state: %v", err)
	}
	if fanState.FanDevice == nil || fanState.FanDevice.ID != "shelly-dimmer-1" {
		t.Fatalf("expected FanDevice shelly-dimmer-1, got %+v", fanState.FanDevice)
	}
	if newFan == nil {
		t.Fatalf("expected fanFactory to be invoked")
	}

	// Unknown fan returns 400
	req = httptest.NewRequest(http.MethodPost, "/api/devices/fan", bytes.NewBufferString(`{"id":"missing"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/devices/fan unknown = %d, want 400", rec.Code)
	}

	// Connect HR monitor
	req = httptest.NewRequest(http.MethodPost, "/api/devices/hr", bytes.NewBufferString(`{"id":"hr-tickr-1"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/devices/hr status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// Wait for streamed HR reading (148 BPM) to arrive in loop
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bridge.Snapshot().BPM == 148 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := bridge.Snapshot().BPM; got != 148 {
		t.Fatalf("expected streamed BPM=148, got %d", got)
	}

	saved, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if saved.BLEDeviceID != "hr-tickr-1" || saved.FanDeviceID != "shelly-dimmer-1" || saved.FanHost != "192.168.1.88:80" {
		t.Fatalf("unexpected saved config: %+v", saved)
	}
	if syncCalls != 2 {
		t.Fatalf("expected 2 onDeviceSync calls, got %d", syncCalls)
	}

	// Stop scan endpoint
	req = httptest.NewRequest(http.MethodPost, "/api/scan/stop", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/scan/stop status = %d, want 200", rec.Code)
	}
}
