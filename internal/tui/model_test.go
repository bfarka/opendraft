package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bfarka/opendraft/internal/config"
	"github.com/bfarka/opendraft/internal/control"
	"github.com/bfarka/opendraft/internal/domain"
)

type stubFan struct {
	dev   domain.FanDevice
	speed int
}

func (f *stubFan) SetSpeed(_ context.Context, p int) error { f.speed = p; return nil }
func (f *stubFan) Off(_ context.Context) error             { f.speed = 0; return nil }
func (f *stubFan) Info() domain.FanDevice                  { return f.dev }

func TestDiscoveryToDashboardAndManualOverride(t *testing.T) {
	fan := &stubFan{}
	m := NewModel(
		context.Background(),
		nil,
		nil,
		func(dev domain.FanDevice) domain.FanController {
			fan.dev = dev
			return fan
		},
		"",
		config.Default(),
	)

	// Receive discovered devices.
	next, _ := m.Update(bleDeviceMsg{ID: "ble-1", Name: "TICKR 1234", RSSI: -55})
	m = next.(Model)
	next, _ = m.Update(fanDeviceMsg{ID: "shellydimmer2-1", Name: "shellydimmer2-1 (Shelly)", IP: "192.168.1.80", Port: 80, Model: "shelly"})
	m = next.(Model)

	if !strings.Contains(m.View(), "TICKR 1234") || !strings.Contains(m.View(), "shellydimmer2-1") {
		t.Fatalf("expected discovered devices in view:\n%s", m.View())
	}

	// Switch to Dimmer pane and select it.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.selectedFan == nil || m.selectedFan.ID != "shellydimmer2-1" {
		t.Fatalf("expected shellydimmer2-1 selected")
	}

	// Switch to Dashboard view.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	if m.view != viewDashboard {
		t.Fatalf("expected viewDashboard")
	}

	// Initially stopped: Start button should be shown.
	if !strings.Contains(m.View(), "START FAN") {
		t.Fatalf("expected START FAN button in dashboard:\n%s", m.View())
	}

	// Press space to START, then +5% manual speed applies to fan.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(Model)
	if !m.loop.State().Running || !strings.Contains(m.View(), "STOP FAN") {
		t.Fatalf("expected STOP FAN button after start:\n%s", m.View())
	}

	// Manual +5% increases speed from 15% (entry speed on Start) to 20% and switches to MANUAL.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	m = next.(Model)
	snap := m.loop.State()
	if snap.Mode != control.ModeManual || snap.AppliedSpeed != 20 || fan.speed != 20 {
		t.Fatalf("expected MANUAL speed 20, got %+v (fan=%d)", snap, fan.speed)
	}

	// Press space to STOP turns fan off.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(Model)
	if m.loop.State().Running || fan.speed != 0 {
		t.Fatalf("expected stopped fan=0, got running=%v fan=%d", m.loop.State().Running, fan.speed)
	}

	// Toggle back to AUTO.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(Model)
	if m.loop.State().Mode != control.ModeAuto {
		t.Fatalf("expected ModeAuto")
	}
}

func TestScansStartedReceivesMultipleDevices(t *testing.T) {
	m := NewModel(context.Background(), nil, nil, nil, "", config.Default())
	fanCh := make(chan domain.FanDevice, 2)
	fanCh <- domain.FanDevice{ID: "s1", Name: "Shelly 1"}
	fanCh <- domain.FanDevice{ID: "s2", Name: "Shelly 2"}
	close(fanCh)

	next, cmd := m.Update(scansStartedMsg{fanCh: fanCh})
	m = next.(Model)
	if cmd == nil || !m.fanScanning {
		t.Fatalf("expected fanScanning=true and non-nil cmd")
	}

	msg1 := cmd()
	next, cmd = m.Update(msg1)
	m = next.(Model)

	msg2 := cmd()
	next, cmd = m.Update(msg2)
	m = next.(Model)

	if len(m.fanDevices) != 2 {
		t.Fatalf("expected 2 fan devices, got %d", len(m.fanDevices))
	}

	msg3 := cmd()
	next, _ = m.Update(msg3)
	m = next.(Model)
	if m.fanScanning {
		t.Fatalf("expected fanScanning=false after channel closed")
	}
}

func TestStartupWithSavedHardwareOpensDashboard(t *testing.T) {
	cfg := config.Default()
	cfg.BLEDeviceID = "ble-saved"
	cfg.BLEDeviceName = "TICKR Saved"
	cfg.FanDeviceID = "shellydimmer2-1"
	cfg.FanDeviceName = "Shelly Dimmer 2"
	cfg.FanHost = "192.168.1.80:80"
	cfg.FanModel = "shelly"

	fan := &stubFan{}
	m := NewModel(
		context.Background(),
		nil,
		nil,
		func(dev domain.FanDevice) domain.FanController {
			fan.dev = dev
			return fan
		},
		"",
		cfg,
	)

	if m.view != viewDashboard {
		t.Fatalf("expected startup on viewDashboard when saved devices exist")
	}
	if m.selectedFan == nil || m.selectedFan.ID != "shellydimmer2-1" || m.selectedFan.IP != "192.168.1.80" {
		t.Fatalf("expected saved fan preloaded, got %+v", m.selectedFan)
	}
	if !strings.Contains(m.View(), "SCAN DEVICES") {
		t.Fatalf("expected SCAN DEVICES button on dashboard:\n%s", m.View())
	}

	// Tab focuses SCAN DEVICES button, Enter activates it -> switches to viewDiscovery.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.buttonFocus != 1 {
		t.Fatalf("expected buttonFocus=1 after Tab, got %d", m.buttonFocus)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.view != viewDiscovery {
		t.Fatalf("expected viewDiscovery after pressing focused SCAN DEVICES button")
	}
}

func TestDashboardChartsAndCurveEditorWindowPercentile(t *testing.T) {
	cfg := config.Default()
	cfg.BLEDeviceID = "ble-1"
	cfg.FanDeviceID = "fan-1"
	cfg.FanHost = "192.168.1.50:80"

	m := NewModel(context.Background(), nil, nil, nil, "", cfg)
	if m.view != viewDashboard {
		t.Fatalf("expected viewDashboard")
	}

	// Send HR readings and verify dashboard renders both charts and P80 readout.
	next, _ := m.Update(hrReadingMsg(domain.HRReading{BPM: 145}))
	m = next.(Model)
	view := m.View()
	if !strings.Contains(view, "HR History (120s)") || !strings.Contains(view, "Fan Curve") || !strings.Contains(view, "P80") {
		t.Fatalf("expected HR history chart, fan curve plot, and P80 in dashboard:\n%s", view)
	}

	// Open curve editor, verify 6 fields, and save updated WindowSeconds/Percentile.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(Model)
	if !m.editingCurve {
		t.Fatalf("expected editingCurve=true")
	}
	if !strings.Contains(m.View(), "Window (seconds, 10-600)") || !strings.Contains(m.View(), "Percentile (%, 1-100)") {
		t.Fatalf("expected window & percentile fields in curve editor:\n%s", m.View())
	}

	// Navigate to Window field (index 4), replace with 60, then Tab to Percentile (index 5), replace with 75, and Enter.
	for i := 0; i < 4; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
	}
	for i := 0; i < 3; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6', '0'}})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	for i := 0; i < 2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7', '5'}})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.editingCurve {
		t.Fatalf("expected editingCurve=false after Enter, errMsg=%q", m.errMsg)
	}
	if m.cfg.WindowSeconds != 60 || m.cfg.Percentile != 75 {
		t.Fatalf("expected WindowSeconds=60, Percentile=75, got %d / %d", m.cfg.WindowSeconds, m.cfg.Percentile)
	}
	snap := m.loop.State()
	if snap.WindowSeconds != 60 || snap.Percentile != 75 {
		t.Fatalf("expected loop updated to 60s / P75, got %+v", snap)
	}
}

func TestVersionShownInViews(t *testing.T) {
	m := NewModel(context.Background(), nil, nil, nil, "", config.Default())
	if !strings.Contains(m.View(), "opendraft (dev)") {
		t.Fatalf("expected default dev version in discovery view:\n%s", m.View())
	}

	m.version = "v1.2.3"
	if !strings.Contains(m.View(), "opendraft (v1.2.3)") {
		t.Fatalf("expected explicit version in discovery view:\n%s", m.View())
	}

	m.view = viewDashboard
	if !strings.Contains(m.View(), "opendraft (v1.2.3)") {
		t.Fatalf("expected explicit version in dashboard view:\n%s", m.View())
	}
}
