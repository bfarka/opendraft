package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bfarka/opendraft/internal/config"
	"github.com/bfarka/opendraft/internal/control"
	"github.com/bfarka/opendraft/internal/domain"
	"github.com/bfarka/opendraft/internal/version"
	"github.com/bfarka/opendraft/internal/web"
)
)

type viewState int

const (
	viewDiscovery viewState = iota
	viewDashboard
)

type discoveryPane int

const (
	paneBLE discoveryPane = iota
	paneDimmer
)

// FanFactory constructs a FanController for a discovered FanDevice.
type FanFactory func(dev domain.FanDevice) domain.FanController

// Messages
type bleDeviceMsg domain.HRDevice
type bleScanDoneMsg struct{}
type bleScanErrMsg struct{ err error }

type fanDeviceMsg domain.FanDevice
type fanScanDoneMsg struct{}
type fanScanErrMsg struct{ err error }

type hrConnectedMsg struct {
	dev    domain.HRDevice
	ch     <-chan domain.HRReading
	cancel context.CancelFunc
}
type hrConnectErrMsg struct{ err error }
type hrReadingMsg domain.HRReading
type hrClosedMsg struct{}

type tickMsg time.Time

// WebDeviceSyncMsg updates TUI device state when a device is connected or selected from the WebUI.
type WebDeviceSyncMsg struct {
	HR      *domain.HRDevice
	Fan     *domain.FanDevice
	Cfg     config.Config
	FanCtrl domain.FanController
}

type scansStartedMsg struct {
	bleCh  <-chan domain.HRDevice
	bleErr error
	fanCh  <-chan domain.FanDevice
	fanErr error
	cancel context.CancelFunc
}

// Model is the root Bubble Tea model for opendraft.
type Model struct {
	ctx        context.Context
	hrSource   domain.HeartRateSource
	fanDisc    domain.FanDiscoverer
	fanFactory FanFactory
	cfgPath    string
	cfg        config.Config
	version    string

	view       viewState
	activePane discoveryPane
	width      int
	height     int

	// Discovery state
	bleDevices  []domain.HRDevice
	bleCursor   int
	bleScanning bool
	fanDevices  []domain.FanDevice
	fanCursor   int
	fanScanning bool
	scanCancel  context.CancelFunc
	bleScanCh   <-chan domain.HRDevice
	fanScanCh   <-chan domain.FanDevice

	// Connected state
	selectedHR   *domain.HRDevice
	selectedFan  *domain.FanDevice
	fanCtrl      domain.FanController
	hrCancel     context.CancelFunc
	hrCh         <-chan domain.HRReading
	loop         *control.Loop
	buttonFocus  int
	editingCurve bool
	curveCursor  int
	curveInputs  [6]string
	statusMsg    string
	errMsg       string
	webBridge    *web.Bridge
	webURL       string
}

// NewModel initializes the TUI model.
func NewModel(
	ctx context.Context,
	hrSource domain.HeartRateSource,
	fanDisc domain.FanDiscoverer,
	fanFactory FanFactory,
	cfgPath string,
	cfg config.Config,
) Model {
	mapper := control.LinearMapper{
		MinHR:    cfg.MinHR,
		MaxHR:    cfg.MaxHR,
		MinSpeed: cfg.MinSpeed,
		MaxSpeed: cfg.MaxSpeed,
	}
	m := Model{
		ctx:        ctx,
		hrSource:   hrSource,
		fanDisc:    fanDisc,
		fanFactory: fanFactory,
		cfgPath:    cfgPath,
		cfg:        cfg,
		version:    version.String(),
		view:       viewDiscovery,
		activePane: paneBLE,
		loop:       control.NewLoop(mapper, nil),
		statusMsg:  "Scanning for BLE HR monitors and Shelly Dimmers...",
	}
	if cfg.HasSavedDevices() {
		fanDev := cfg.SavedFanDevice()
		m.view = viewDashboard
		m.selectedFan = &fanDev
		if fanFactory != nil {
			m.fanCtrl = fanFactory(fanDev)
		}
		m.loop = control.NewLoop(mapper, m.fanCtrl)
		hrLabel := cfg.BLEDeviceName
		if hrLabel == "" {
			hrLabel = cfg.BLEDeviceID
		}
		m.statusMsg = fmt.Sprintf("Loaded saved devices; reconnecting to %s...", hrLabel)
	}
	m.loop.SetWindowConfig(ctx, cfg.WindowSeconds, cfg.Percentile)
	return m
}

// WithWebBridge attaches a web.Bridge so TUI state changes are mirrored to the WebUI.
func (m Model) WithWebBridge(b *web.Bridge) Model {
	m.webBridge = b
	if b != nil {
		b.SetLoop(m.loop)
		b.SetDevices(m.selectedHR, m.selectedFan)
		b.SetConfig(m.cfg)
		b.SetStatus(m.statusMsg, m.errMsg)
	}
	return m
}

// WithWebURL sets the local WebUI URL displayed in the TUI.
func (m Model) WithWebURL(url string) Model {
	m.webURL = url
	return m
}

func (m *Model) notifyWeb() {
	if m.webBridge != nil {
		m.webBridge.Notify()
	}
}

// Init starts initial BLE and mDNS discovery scans and the 1s watchdog ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startScansCmd(m.ctx, m.hrSource, m.fanDisc),
		tickEverySecond(),
	)
}

func tickEverySecond() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func startScansCmd(parent context.Context, hrSource domain.HeartRateSource, fanDisc domain.FanDiscoverer) tea.Cmd {
	return func() tea.Msg {
		scanCtx, cancel := context.WithTimeout(parent, 15*time.Second)
		res := scansStartedMsg{cancel: cancel}
		if hrSource != nil {
			res.bleCh, res.bleErr = hrSource.Scan(scanCtx)
		}
		if fanDisc != nil {
			res.fanCh, res.fanErr = fanDisc.Scan(scanCtx)
		}
		return res
	}
}

func waitBLEDevice(ch <-chan domain.HRDevice) tea.Cmd {
	return func() tea.Msg {
		dev, ok := <-ch
		if !ok {
			return bleScanDoneMsg{}
		}
		return bleDeviceMsg(dev)
	}
}

func waitFanDevice(ch <-chan domain.FanDevice) tea.Cmd {
	return func() tea.Msg {
		dev, ok := <-ch
		if !ok {
			return fanScanDoneMsg{}
		}
		return fanDeviceMsg(dev)
	}
}

func waitHRReading(ch <-chan domain.HRReading) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return hrClosedMsg{}
		}
		return hrReadingMsg(r)
	}
}

func (m *Model) connectHR(dev domain.HRDevice) tea.Cmd {
	if m.webBridge != nil {
		m.webBridge.CancelHRStream()
	}
	if m.hrCancel != nil {
		m.hrCancel()
	}
	if m.scanCancel != nil {
		m.scanCancel()
		m.bleScanning = false
		m.fanScanning = false
	}
	connCtx, cancel := context.WithCancel(m.ctx)
	return func() tea.Msg {
		ch, err := m.hrSource.Connect(connCtx, dev.ID)
		if err != nil {
			cancel()
			return hrConnectErrMsg{err: err}
		}
		return hrConnectedMsg{dev: dev, ch: ch, cancel: cancel}
	}
}

func (m *Model) rebuildLoop() {
	mapper := control.LinearMapper{
		MinHR:    m.cfg.MinHR,
		MaxHR:    m.cfg.MaxHR,
		MinSpeed: m.cfg.MinSpeed,
		MaxSpeed: m.cfg.MaxSpeed,
	}
	m.loop = control.NewLoop(mapper, m.fanCtrl)
	m.loop.SetWindowConfig(m.ctx, m.cfg.WindowSeconds, m.cfg.Percentile)
	if m.webBridge != nil {
		m.webBridge.SetLoop(m.loop)
	}
}

func (m *Model) saveConfig() {
	if m.cfgPath == "" {
		return
	}
	_ = config.Save(m.cfgPath, m.cfg)
}

// Update handles Bubble Tea messages and key events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scansStartedMsg:
		if m.scanCancel != nil {
			m.scanCancel()
		}
		m.scanCancel = msg.cancel
		m.bleDevices = nil
		m.fanDevices = nil
		m.bleCursor = 0
		m.fanCursor = 0
		var cmds []tea.Cmd
		if msg.bleErr != nil {
			m.bleScanning = false
			m.errMsg = fmt.Sprintf("BLE scan: %v", msg.bleErr)
		} else if msg.bleCh != nil {
			m.bleScanning = true
			m.bleScanCh = msg.bleCh
			cmds = append(cmds, waitBLEDevice(msg.bleCh))
		} else {
			m.bleScanning = false
		}
		if msg.fanErr != nil {
			m.fanScanning = false
			m.errMsg = fmt.Sprintf("mDNS scan: %v", msg.fanErr)
		} else if msg.fanCh != nil {
			m.fanScanning = true
			m.fanScanCh = msg.fanCh
			cmds = append(cmds, waitFanDevice(msg.fanCh))
		} else {
			m.fanScanning = false
		}
		return m, tea.Batch(cmds...)

	case bleDeviceMsg:
		dev := domain.HRDevice(msg)
		m.bleDevices = append(m.bleDevices, dev)
		if m.view == viewDashboard && m.selectedHR == nil && m.cfg.BLEDeviceID != "" && dev.ID == m.cfg.BLEDeviceID && m.hrSource != nil {
			m.statusMsg = fmt.Sprintf("Reconnecting to %s...", dev.Name)
			return m, m.connectHR(dev)
		}
		if m.bleScanCh != nil {
			return m, waitBLEDevice(m.bleScanCh)
		}
		return m, nil

	case bleScanDoneMsg:
		m.bleScanning = false
		return m, nil

	case bleScanErrMsg:
		m.bleScanning = false
		m.errMsg = fmt.Sprintf("BLE scan: %v", msg.err)
		return m, nil

	case fanDeviceMsg:
		m.fanDevices = append(m.fanDevices, domain.FanDevice(msg))
		if m.fanScanCh != nil {
			return m, waitFanDevice(m.fanScanCh)
		}
		return m, nil

	case fanScanDoneMsg:
		m.fanScanning = false
		return m, nil

	case fanScanErrMsg:
		m.fanScanning = false
		m.errMsg = fmt.Sprintf("mDNS scan: %v", msg.err)
		return m, nil

	case hrConnectedMsg:
		m.selectedHR = &msg.dev
		m.hrCh = msg.ch
		m.hrCancel = msg.cancel
		m.cfg.BLEDeviceID = msg.dev.ID
		m.cfg.BLEDeviceName = msg.dev.Name
		m.saveConfig()
		m.errMsg = ""
		m.statusMsg = fmt.Sprintf("Connected HR: %s", msg.dev.Name)
		if m.webBridge != nil {
			m.webBridge.SetDevices(m.selectedHR, m.selectedFan)
			m.webBridge.SetConfig(m.cfg)
			m.webBridge.SetStatus(m.statusMsg, m.errMsg)
		}
		if m.selectedFan != nil {
			m.view = viewDashboard
		}
		return m, waitHRReading(msg.ch)

	case hrConnectErrMsg:
		m.errMsg = fmt.Sprintf("BLE connect: %v", msg.err)
		return m, nil

	case hrReadingMsg:
		m.loop.HandleHR(m.ctx, domain.HRReading(msg))
		m.notifyWeb()
		if m.hrCh != nil {
			return m, waitHRReading(m.hrCh)
		}
		return m, nil

	case hrClosedMsg:
		m.statusMsg = "HR stream closed"
		m.notifyWeb()
		return m, nil

	case tickMsg:
		m.loop.CheckTimeout(m.ctx)
		m.notifyWeb()
		return m, tickEverySecond()

	case WebDeviceSyncMsg:
		if msg.HR != nil {
			if m.hrCancel != nil {
				m.hrCancel()
				m.hrCancel = nil
				m.hrCh = nil
			}
			cp := *msg.HR
			m.selectedHR = &cp
		}
		if msg.Fan != nil {
			cp := *msg.Fan
			m.selectedFan = &cp
		}
		if msg.FanCtrl != nil {
			m.fanCtrl = msg.FanCtrl
		}
		m.cfg = msg.Cfg
		if m.selectedHR != nil && m.selectedFan != nil {
			m.view = viewDashboard
		}
		return m, nil

	case tea.KeyMsg:
		if m.view == viewDiscovery {
			return m.updateDiscoveryKey(msg)
		}
		return m.updateDashboardKey(msg)

	case tea.MouseMsg:
		if m.view == viewDashboard && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleDashboardClick(msg)
		}
		return m, nil
	}

	return m, nil
}

// View renders the active TUI screen.
func (m Model) View() string {
	if m.view == viewDiscovery {
		return m.viewDiscoveryScreen()
	}
	return m.viewDashboardScreen()
}

func (m Model) renderHeader(screen string) string {
	v := m.version
	if v == "" {
		v = "dev"
	}
	return titleStyle.Render(fmt.Sprintf("opendraft (%s) — %s", v, screen))
}
