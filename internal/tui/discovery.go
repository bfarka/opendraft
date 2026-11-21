package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateDiscoveryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.fanCtrl != nil {
			_ = m.fanCtrl.Off(m.ctx)
		}
		return m, tea.Quit

	case "tab", "left", "right":
		if m.activePane == paneBLE {
			m.activePane = paneDimmer
		} else {
			m.activePane = paneBLE
		}

	case "up", "k":
		if m.activePane == paneBLE && m.bleCursor > 0 {
			m.bleCursor--
		} else if m.activePane == paneDimmer && m.fanCursor > 0 {
			m.fanCursor--
		}

	case "down", "j":
		if m.activePane == paneBLE && m.bleCursor < len(m.bleDevices)-1 {
			m.bleCursor++
		} else if m.activePane == paneDimmer && m.fanCursor < len(m.fanDevices)-1 {
			m.fanCursor++
		}

	case "r":
		m.errMsg = ""
		m.statusMsg = "Rescanning BLE and Shelly mDNS..."
		return m, startScansCmd(m.ctx, m.hrSource, m.fanDisc)

	case "d":
		m.view = viewDashboard

	case "enter":
		if m.activePane == paneBLE && len(m.bleDevices) > 0 {
			dev := m.bleDevices[m.bleCursor]
			m.statusMsg = fmt.Sprintf("Connecting to %s...", dev.Name)
			return m, m.connectHR(dev)
		}
		if m.activePane == paneDimmer && len(m.fanDevices) > 0 {
			dev := m.fanDevices[m.fanCursor]
			m.selectedFan = &dev
			if m.fanFactory != nil {
				m.fanCtrl = m.fanFactory(dev)
				m.rebuildLoop()
			}
			m.cfg.FanDeviceID = dev.ID
			m.cfg.FanDeviceName = dev.Name
			m.cfg.FanModel = dev.Model
			m.cfg.FanHost = fmt.Sprintf("%s:%d", dev.IP, dev.Port)
			m.saveConfig()
			m.statusMsg = fmt.Sprintf("Selected Shelly: %s", dev.Name)
			if m.webBridge != nil {
				m.webBridge.SetDevices(m.selectedHR, m.selectedFan)
				m.webBridge.SetConfig(m.cfg)
				m.webBridge.SetStatus(m.statusMsg, m.errMsg)
			}
			if m.selectedHR != nil {
				m.view = viewDashboard
			}
		}
	}

	return m, nil
}

func (m Model) viewDiscoveryScreen() string {
	var b strings.Builder
	b.WriteString(m.renderHeader("Device Discovery"))
	b.WriteString("\n")

	paneWidth := 42
	if m.width > 90 {
		paneWidth = (m.width - 6) / 2
	}

	bleTitle := "BLE Heart Rate Monitors"
	if m.bleScanning {
		bleTitle += " (scanning...)"
	}
	var bleLines []string
	bleLines = append(bleLines, bleTitle, "")
	if len(m.bleDevices) == 0 {
		bleLines = append(bleLines, mutedStyle.Render("No BLE HR devices found yet"))
	} else {
		for i, d := range m.bleDevices {
			prefix := "  "
			if i == m.bleCursor && m.activePane == paneBLE {
				prefix = "> "
			}
			marker := ""
			if m.selectedHR != nil && m.selectedHR.ID == d.ID {
				snap := m.loop.State()
				if snap.HRConnected {
					marker = fmt.Sprintf(" [connected: %d bpm]", snap.BPM)
				} else {
					marker = " [connected]"
				}
			}
			line := fmt.Sprintf("%s%s (%ddBm)%s", prefix, d.Name, d.RSSI, marker)
			if i == m.bleCursor && m.activePane == paneBLE {
				line = selectedRowStyle.Render(line)
			}
			bleLines = append(bleLines, line)
		}
	}

	fanTitle := "Shelly Dimmers (mDNS)"
	if m.fanScanning {
		fanTitle += " (scanning...)"
	}
	var fanLines []string
	fanLines = append(fanLines, fanTitle, "")
	if len(m.fanDevices) == 0 {
		fanLines = append(fanLines, mutedStyle.Render("No Shelly dimmers found yet"))
	} else {
		for i, d := range m.fanDevices {
			prefix := "  "
			if i == m.fanCursor && m.activePane == paneDimmer {
				prefix = "> "
			}
			marker := ""
			if m.selectedFan != nil && m.selectedFan.ID == d.ID {
				marker = " [selected]"
			}
			line := fmt.Sprintf("%s%s @ %s:%d%s", prefix, d.Name, d.IP, d.Port, marker)
			if i == m.fanCursor && m.activePane == paneDimmer {
				line = selectedRowStyle.Render(line)
			}
			fanLines = append(fanLines, line)
		}
	}

	bleBoxStyle := paneStyle.Width(paneWidth)
	fanBoxStyle := paneStyle.Width(paneWidth)
	if m.activePane == paneBLE {
		bleBoxStyle = activePaneStyle.Width(paneWidth)
	} else {
		fanBoxStyle = activePaneStyle.Width(paneWidth)
	}

	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		bleBoxStyle.Render(strings.Join(bleLines, "\n")),
		"  ",
		fanBoxStyle.Render(strings.Join(fanLines, "\n")),
	)
	b.WriteString(row)
	b.WriteString("\n\n")

	if m.errMsg != "" {
		b.WriteString(errStyle.Render("Error: "+m.errMsg) + "\n")
	} else if m.statusMsg != "" {
		b.WriteString(mutedStyle.Render(m.statusMsg) + "\n")
	}

	if m.webURL != "" {
		b.WriteString(mutedStyle.Render("WebUI: "+m.webURL) + "\n")
	}

	b.WriteString(mutedStyle.Render("tab: switch pane • ↑/↓: select • enter: connect • r: rescan • d: dashboard • q: quit"))
	return b.String()
}
