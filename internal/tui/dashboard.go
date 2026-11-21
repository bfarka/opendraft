package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bfarka/opendraft/internal/control"
)

var curveFieldLabels = [6]string{
	"Entry BPM (fixed min fan up to this HR)",
	"Entry Fan % (fixed % at/below Entry BPM)",
	"Max BPM (100% / Max Fan at this HR)",
	"Max Fan %",
	"Window (seconds, 10-600)",
	"Percentile (%, 1-100)",
}

func (m *Model) openCurveEditor() {
	if m.webBridge != nil {
		m.cfg = m.webBridge.Config()
	}
	m.editingCurve = true
	m.curveCursor = 0
	m.curveInputs = [6]string{
		strconv.Itoa(int(m.cfg.MinHR)),
		strconv.Itoa(m.cfg.MinSpeed),
		strconv.Itoa(int(m.cfg.MaxHR)),
		strconv.Itoa(m.cfg.MaxSpeed),
		strconv.Itoa(m.cfg.WindowSeconds),
		strconv.Itoa(m.cfg.Percentile),
	}
}

func (m Model) updateCurveEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.editingCurve = false
		return m, nil
	case tea.KeyUp:
		if m.curveCursor > 0 {
			m.curveCursor--
		}
		return m, nil
	case tea.KeyDown, tea.KeyTab:
		m.curveCursor = (m.curveCursor + 1) % len(curveFieldLabels)
		return m, nil
	case tea.KeyBackspace:
		cur := m.curveInputs[m.curveCursor]
		if len(cur) > 0 {
			m.curveInputs[m.curveCursor] = cur[:len(cur)-1]
		}
		return m, nil
	case tea.KeyEnter:
		minHR, err1 := strconv.Atoi(m.curveInputs[0])
		minSpd, err2 := strconv.Atoi(m.curveInputs[1])
		maxHR, err3 := strconv.Atoi(m.curveInputs[2])
		maxSpd, err4 := strconv.Atoi(m.curveInputs[3])
		winSec, err5 := strconv.Atoi(m.curveInputs[4])
		pct, err6 := strconv.Atoi(m.curveInputs[5])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil ||
			minHR < 40 || minHR > 220 || maxHR <= minHR || maxHR > 240 ||
			minSpd < 0 || minSpd > 100 || maxSpd < minSpd || maxSpd > 100 ||
			winSec < 10 || winSec > 600 || pct < 1 || pct > 100 {
			m.errMsg = "Invalid values (BPM 40-240, Fan 0-100%, Window 10-600s, P 1-100%)"
			return m, nil
		}
		m.cfg.MinHR = uint8(minHR)
		m.cfg.MinSpeed = minSpd
		m.cfg.MaxHR = uint8(maxHR)
		m.cfg.MaxSpeed = maxSpd
		m.cfg.WindowSeconds = winSec
		m.cfg.Percentile = pct
		m.saveConfig()
		m.loop.SetMapper(m.ctx, control.LinearMapper{
			MinHR:    m.cfg.MinHR,
			MaxHR:    m.cfg.MaxHR,
			MinSpeed: m.cfg.MinSpeed,
			MaxSpeed: m.cfg.MaxSpeed,
		})
		m.loop.SetWindowConfig(m.ctx, m.cfg.WindowSeconds, m.cfg.Percentile)
		if m.webBridge != nil {
			m.webBridge.SetConfig(m.cfg)
		}
		m.editingCurve = false
		m.errMsg = ""
		m.statusMsg = fmt.Sprintf("Saved curve: <=%d bpm -> %d%%, %d bpm -> %d%% (P%d/%ds)",
			m.cfg.MinHR, m.cfg.MinSpeed, m.cfg.MaxHR, m.cfg.MaxSpeed, m.cfg.Percentile, m.cfg.WindowSeconds)
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				if r >= '0' && r <= '9' && len(m.curveInputs[m.curveCursor]) < 3 {
					m.curveInputs[m.curveCursor] += string(r)
				}
			}
		}
		return m, nil
	}
}

func (m Model) handleDashboardClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Buttons sit below the info box (rows ~11..15).
	if msg.Y >= 10 && msg.Y <= 16 {
		if msg.X >= 0 && msg.X <= 24 {
			if m.loop.State().Running {
				m.loop.Stop(m.ctx)
			} else {
				m.loop.Start(m.ctx)
			}
		} else if msg.X >= 25 && msg.X <= 48 {
			m.view = viewDiscovery
			m.statusMsg = "Scanning for BLE HR monitors and Shelly Dimmers..."
			return m, startScansCmd(m.ctx, m.hrSource, m.fanDisc)
		}
	}
	return m, nil
}

func (m Model) updateDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editingCurve {
		return m.updateCurveEditorKey(msg)
	}

	snap := m.loop.State()

	switch msg.String() {
	case "q", "ctrl+c":
		m.loop.Stop(m.ctx)
		if m.fanCtrl != nil {
			_ = m.fanCtrl.Off(m.ctx)
		}
		return m, tea.Quit

	case "tab", "left", "right":
		m.buttonFocus = 1 - m.buttonFocus

	case " ", "enter":
		if m.buttonFocus == 1 {
			m.view = viewDiscovery
			m.statusMsg = "Scanning for BLE HR monitors and Shelly Dimmers..."
			return m, startScansCmd(m.ctx, m.hrSource, m.fanDisc)
		}
		if snap.Running {
			m.loop.Stop(m.ctx)
		} else {
			m.loop.Start(m.ctx)
		}
		m.notifyWeb()

	case "c":
		m.openCurveEditor()

	case "s":
		m.view = viewDiscovery
		m.statusMsg = "Scanning for BLE HR monitors and Shelly Dimmers..."
		return m, startScansCmd(m.ctx, m.hrSource, m.fanDisc)

	case "a":
		if snap.Mode == control.ModeAuto {
			m.loop.SetMode(m.ctx, control.ModeManual)
		} else {
			m.loop.SetMode(m.ctx, control.ModeAuto)
		}
		m.notifyWeb()

	case "+", "=", "up":
		m.loop.SetManualSpeed(m.ctx, snap.AppliedSpeed+5)
		m.notifyWeb()

	case "-", "_", "down":
		m.loop.SetManualSpeed(m.ctx, snap.AppliedSpeed-5)
		m.notifyWeb()

	case "0":
		m.loop.SetManualSpeed(m.ctx, 0)
		m.notifyWeb()
	}

	return m, nil
}

func (m Model) viewDashboardScreen() string {
	snap := m.loop.State()

	var b strings.Builder
	b.WriteString(m.renderHeader("Live Dashboard"))
	b.WriteString("\n")

	hrName := "not connected"
	if m.selectedHR != nil {
		hrName = m.selectedHR.Name
	}
	fanName := "not selected"
	if m.selectedFan != nil {
		fanName = fmt.Sprintf("%s (%s:%d)", m.selectedFan.Name, m.selectedFan.IP, m.selectedFan.Port)
	}

	modeBadge := autoBadgeStyle.Render(string(snap.Mode))
	if snap.Mode == control.ModeManual {
		modeBadge = manualBadgeStyle.Render(string(snap.Mode))
	}

	hrDisplay := "-- bpm (waiting for data)"
	if snap.HRConnected {
		hrDisplay = fmt.Sprintf("%d bpm  (P%d %ds: %d bpm, %d/%ds)",
			snap.BPM, snap.Percentile, snap.WindowSeconds, snap.EffectiveBPM, snap.WindowCount, snap.WindowSeconds)
	}

	bar := renderSpeedBar(snap.AppliedSpeed, 25)

	lines := []string{
		fmt.Sprintf("HR Monitor : %s", hrName),
		fmt.Sprintf("Dimmer Fan : %s", fanName),
		"",
		fmt.Sprintf("Heart Rate : %s", selectedRowStyle.Render(hrDisplay)),
		fmt.Sprintf("Mode       : %s", modeBadge),
		fmt.Sprintf("Fan Speed  : %3d%%  %s (target %d%%)", snap.AppliedSpeed, bar, snap.TargetSpeed),
		fmt.Sprintf("HR Curve   : <=%d bpm -> %d%%  |  %d+ bpm -> %d%%  (P%d/%ds)",
			m.cfg.MinHR, m.cfg.MinSpeed, m.cfg.MaxHR, m.cfg.MaxSpeed, m.cfg.Percentile, m.cfg.WindowSeconds),
	}
	if m.webURL != "" {
		lines = append(lines, fmt.Sprintf("WebUI      : %s", m.webURL))
	}

	b.WriteString(activePaneStyle.Width(64).Render(strings.Join(lines, "\n")))
	b.WriteString("\n")

	startLabel := "  ▶   START FAN  "
	if snap.Running {
		startLabel = "  ■   STOP FAN   "
	}
	scanLabel := "  🔍  SCAN DEVICES  "
	if m.buttonFocus == 0 {
		startLabel = "[ " + strings.TrimSpace(startLabel) + " ]"
	} else {
		scanLabel = "[ " + strings.TrimSpace(scanLabel) + " ]"
	}

	var startStopBtn string
	if snap.Running {
		startStopBtn = stopButtonStyle.Render(startLabel)
	} else {
		startStopBtn = startButtonStyle.Render(startLabel)
	}
	scanBtn := scanButtonStyle.Render(scanLabel)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, startStopBtn, "  ", scanBtn))
	b.WriteString("\n\n")

	history := m.loop.History(nil)
	histChart := paneStyle.Width(64).Render(
		renderHRHistoryChart(history, snap.BPM, snap.EffectiveBPM, snap.WindowSeconds, snap.Percentile, m.cfg.MinHR, m.cfg.MaxHR, 52, 5),
	)
	mapper := control.LinearMapper{
		MinHR:    m.cfg.MinHR,
		MaxHR:    m.cfg.MaxHR,
		MinSpeed: m.cfg.MinSpeed,
		MaxSpeed: m.cfg.MaxSpeed,
	}
	curveChart := paneStyle.Width(64).Render(
		renderCurvePlot(mapper, snap.BPM, snap.EffectiveBPM, snap.Percentile, 50, 5),
	)
	if m.width >= 134 {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, histChart, " ", curveChart))
	} else {
		b.WriteString(histChart + "\n" + curveChart)
	}
	b.WriteString("\n\n")

	if m.editingCurve {
		var cLines []string
		cLines = append(cLines, "Edit HR Curve (↑/↓/Tab: field, Enter: save, Esc: cancel)", "")
		for i, label := range curveFieldLabels {
			prefix := "  "
			if i == m.curveCursor {
				prefix = "> "
			}
			line := fmt.Sprintf("%s%-42s : %s", prefix, label, m.curveInputs[i])
			if i == m.curveCursor {
				line = selectedRowStyle.Render(line + "_")
			}
			cLines = append(cLines, line)
		}
		b.WriteString(paneStyle.Width(64).Render(strings.Join(cLines, "\n")))
		b.WriteString("\n\n")
	}

	if snap.LastErr != "" {
		b.WriteString(errStyle.Render("Fan error: "+snap.LastErr) + "\n")
	} else if m.errMsg != "" {
		b.WriteString(errStyle.Render("Error: "+m.errMsg) + "\n")
	} else if m.statusMsg != "" {
		b.WriteString(mutedStyle.Render(m.statusMsg) + "\n")
	}

	b.WriteString(mutedStyle.Render("tab/←/→: select button • space/enter: press • c: edit curve • a: AUTO/MANUAL • +/-: speed • q: quit"))
	return b.String()
}

func renderSpeedBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := (percent * width) / 100
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
