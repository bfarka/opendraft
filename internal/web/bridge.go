package web

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/bfarka/opendraft/internal/config"
	"github.com/bfarka/opendraft/internal/control"
	"github.com/bfarka/opendraft/internal/domain"
	"github.com/bfarka/opendraft/internal/version"
)

// CurveConfig holds the editable HR-to-fan curve and rolling window parameters.
type CurveConfig struct {
	MinHR         uint8 `json:"min_hr"`
	MaxHR         uint8 `json:"max_hr"`
	MinSpeed      int   `json:"min_speed"`
	MaxSpeed      int   `json:"max_speed"`
	WindowSeconds int   `json:"window_seconds"`
	Percentile    int   `json:"percentile"`
}

// DeviceInfo describes a connected or selected HR monitor or fan dimmer.
type DeviceInfo struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Host  string `json:"host,omitempty"`
	Model string `json:"model,omitempty"`
}

// DiscoveredHR describes a BLE heart rate monitor found during scanning.
type DiscoveredHR struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	RSSI int16  `json:"rssi"`
}

// DiscoveredFan describes a Shelly dimmer found during mDNS scanning.
type DiscoveredFan struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IP    string `json:"ip"`
	Port  int    `json:"port"`
	Model string `json:"model"`
}

// DiscoveryState holds active scan flags and discovered device lists.
type DiscoveryState struct {
	BLEScanning bool            `json:"ble_scanning"`
	FanScanning bool            `json:"fan_scanning"`
	HRDevices   []DiscoveredHR  `json:"hr_devices"`
	FanDevices  []DiscoveredFan `json:"fan_devices"`
	BLEError    string          `json:"ble_error,omitempty"`
	FanError    string          `json:"fan_error,omitempty"`
}

// FanFactory constructs a FanController for a selected FanDevice.
type FanFactory func(dev domain.FanDevice) domain.FanController

// State is the JSON-serializable snapshot exposed by the WebUI API and SSE stream.
type State struct {
	Version       string         `json:"version"`
	Running       bool           `json:"running"`
	Mode          string         `json:"mode"`
	BPM           uint8          `json:"bpm"`
	EffectiveBPM  uint8          `json:"effective_bpm"`
	WindowCount   int            `json:"window_count"`
	WindowSeconds int            `json:"window_seconds"`
	Percentile    int            `json:"percentile"`
	TargetSpeed   int            `json:"target_speed"`
	AppliedSpeed  int            `json:"applied_speed"`
	HRConnected   bool           `json:"hr_connected"`
	LastHRAt      *time.Time     `json:"last_hr_at,omitempty"`
	LastErr       string         `json:"last_err,omitempty"`
	History       []int          `json:"history"`
	Curve         CurveConfig    `json:"curve"`
	HRDevice      *DeviceInfo    `json:"hr_device,omitempty"`
	FanDevice     *DeviceInfo    `json:"fan_device,omitempty"`
	Discovery     DiscoveryState `json:"discovery"`
	StatusMsg     string         `json:"status_msg,omitempty"`
	ErrMsg        string         `json:"err_msg,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Bridge coordinates thread-safe state access and control actions between the app and HTTP server.
type Bridge struct {
	mu             sync.RWMutex
	ctx            context.Context
	loop           *control.Loop
	cfgPath        string
	cfg            config.Config
	hrDev          *domain.HRDevice
	fanDev         *domain.FanDevice
	hrSource       domain.HeartRateSource
	fanDisc        domain.FanDiscoverer
	fanFactory     FanFactory
	onDeviceSync   func(hr *domain.HRDevice, fan *domain.FanDevice, cfg config.Config, fanCtrl domain.FanController)
	scanCancel     context.CancelFunc
	scanGen        uint64
	bleScanning    bool
	fanScanning    bool
	bleErr         string
	fanErr         string
	discoveredHR   []domain.HRDevice
	discoveredFan  []domain.FanDevice
	hrStreamCancel context.CancelFunc
	statusMsg      string
	errMsg         string
	subs           map[chan State]struct{}
}

// NewBridge creates a Bridge for the given control loop and configuration.
func NewBridge(ctx context.Context, loop *control.Loop, cfgPath string, cfg config.Config) *Bridge {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Bridge{
		ctx:     ctx,
		loop:    loop,
		cfgPath: cfgPath,
		cfg:     cfg,
		subs:    make(map[chan State]struct{}),
	}
}

// SetLoop updates the active control.Loop reference and notifies subscribers.
func (b *Bridge) SetLoop(loop *control.Loop) {
	b.mu.Lock()
	b.loop = loop
	b.mu.Unlock()
	b.Notify()
}

// SetDevices updates the active HR and fan device metadata and notifies subscribers.
func (b *Bridge) SetDevices(hr *domain.HRDevice, fan *domain.FanDevice) {
	b.mu.Lock()
	if hr != nil {
		cp := *hr
		b.hrDev = &cp
	} else {
		b.hrDev = nil
	}
	if fan != nil {
		cp := *fan
		b.fanDev = &cp
	} else {
		b.fanDev = nil
	}
	b.mu.Unlock()
	b.Notify()
}

// SetConfig updates the stored configuration and notifies subscribers.
func (b *Bridge) SetConfig(cfg config.Config) {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
	b.Notify()
}

// SetStatus updates the current status and error banner messages and notifies subscribers.
func (b *Bridge) SetStatus(statusMsg, errMsg string) {
	b.mu.Lock()
	b.statusMsg = statusMsg
	b.errMsg = errMsg
	b.mu.Unlock()
	b.Notify()
}

// Config returns a copy of the current config.Config.
func (b *Bridge) Config() config.Config {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

// Snapshot builds a point-in-time State from the control loop and metadata.
func (b *Bridge) Snapshot() State {
	b.mu.RLock()
	loop := b.loop
	cfg := b.cfg
	hrDev := b.hrDev
	fanDev := b.fanDev
	statusMsg := b.statusMsg
	errMsg := b.errMsg
	bleScanning := b.bleScanning
	fanScanning := b.fanScanning
	bleErr := b.bleErr
	fanErr := b.fanErr
	discHR := make([]DiscoveredHR, len(b.discoveredHR))
	for i, d := range b.discoveredHR {
		name := d.Name
		if name == "" {
			name = d.ID
		}
		discHR[i] = DiscoveredHR{ID: d.ID, Name: name, RSSI: d.RSSI}
	}
	discFan := make([]DiscoveredFan, len(b.discoveredFan))
	for i, d := range b.discoveredFan {
		name := d.Name
		if name == "" {
			name = d.ID
		}
		discFan[i] = DiscoveredFan{ID: d.ID, Name: name, IP: d.IP, Port: d.Port, Model: d.Model}
	}
	b.mu.RUnlock()

	s := State{
		Version: version.String(),
		Mode:    string(control.ModeAuto),
		Curve: CurveConfig{
			MinHR:         cfg.MinHR,
			MaxHR:         cfg.MaxHR,
			MinSpeed:      cfg.MinSpeed,
			MaxSpeed:      cfg.MaxSpeed,
			WindowSeconds: cfg.WindowSeconds,
			Percentile:    cfg.Percentile,
		},
		Discovery: DiscoveryState{
			BLEScanning: bleScanning,
			FanScanning: fanScanning,
			HRDevices:   discHR,
			FanDevices:  discFan,
			BLEError:    bleErr,
			FanError:    fanErr,
		},
		StatusMsg: statusMsg,
		ErrMsg:    errMsg,
		UpdatedAt: time.Now().UTC(),
	}

	if hrDev != nil {
		name := hrDev.Name
		if name == "" {
			name = hrDev.ID
		}
		s.HRDevice = &DeviceInfo{
			ID:   hrDev.ID,
			Name: name,
		}
	}
	if fanDev != nil {
		name := fanDev.Name
		if name == "" {
			name = fanDev.ID
		}
		host := fanDev.IP
		if host != "" && fanDev.Port > 0 && fanDev.Port != 80 {
			host = net.JoinHostPort(host, strconv.Itoa(fanDev.Port))
		}
		s.FanDevice = &DeviceInfo{
			ID:    fanDev.ID,
			Name:  name,
			Host:  host,
			Model: fanDev.Model,
		}
	}

	if loop != nil {
		snap := loop.State()
		s.Running = snap.Running
		s.Mode = string(snap.Mode)
		s.BPM = snap.BPM
		s.EffectiveBPM = snap.EffectiveBPM
		s.WindowCount = snap.WindowCount
		s.WindowSeconds = snap.WindowSeconds
		s.Percentile = snap.Percentile
		s.TargetSpeed = snap.TargetSpeed
		s.AppliedSpeed = snap.AppliedSpeed
		s.HRConnected = snap.HRConnected
		s.LastErr = snap.LastErr
		if !snap.LastHRAt.IsZero() {
			t := snap.LastHRAt.UTC()
			s.LastHRAt = &t
		}
		raw := loop.History(nil)
		s.History = make([]int, len(raw))
		for i, v := range raw {
			s.History[i] = int(v)
		}
	}
	if s.History == nil {
		s.History = []int{}
	}
	return s
}

// Subscribe registers a buffered channel to receive State updates until the returned cancel func is called.
func (b *Bridge) Subscribe() (<-chan State, func()) {
	ch := make(chan State, 8)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

// Notify broadcasts the latest State to all active SSE subscribers without blocking.
func (b *Bridge) Notify() {
	b.mu.RLock()
	hasSubs := len(b.subs) > 0
	b.mu.RUnlock()
	if !hasSubs {
		return
	}

	st := b.Snapshot()
	b.mu.RLock()
	for ch := range b.subs {
		select {
		case ch <- st:
		default:
			// Drop if subscriber is lagging; next tick/update will catch up.
		}
	}
	b.mu.RUnlock()
}

// Start enables the fan control loop and notifies subscribers.
func (b *Bridge) Start() State {
	b.mu.RLock()
	loop := b.loop
	ctx := b.ctx
	b.mu.RUnlock()

	if loop != nil {
		loop.Start(ctx)
	}
	b.Notify()
	return b.Snapshot()
}

// Stop pauses the fan control loop, turns the fan off, and notifies subscribers.
func (b *Bridge) Stop() State {
	b.mu.RLock()
	loop := b.loop
	ctx := b.ctx
	b.mu.RUnlock()

	if loop != nil {
		loop.Stop(ctx)
	}
	b.Notify()
	return b.Snapshot()
}

// SetMode switches between AUTO and MANUAL modes.
func (b *Bridge) SetMode(mode control.Mode) (State, error) {
	if mode != control.ModeAuto && mode != control.ModeManual {
		return State{}, fmt.Errorf("invalid mode %q: must be AUTO or MANUAL", mode)
	}
	b.mu.RLock()
	loop := b.loop
	ctx := b.ctx
	b.mu.RUnlock()

	if loop != nil {
		loop.SetMode(ctx, mode)
	}
	b.Notify()
	return b.Snapshot(), nil
}

// SetManualSpeed switches to MANUAL mode and sets the target speed (0..100).
func (b *Bridge) SetManualSpeed(speed int) (State, error) {
	if speed < 0 || speed > 100 {
		return State{}, fmt.Errorf("invalid speed %d: must be 0..100", speed)
	}
	b.mu.RLock()
	loop := b.loop
	ctx := b.ctx
	b.mu.RUnlock()

	if loop != nil {
		loop.SetManualSpeed(ctx, speed)
	}
	b.Notify()
	return b.Snapshot(), nil
}

// UpdateCurve validates, persists, and applies new HR-to-fan curve and window settings.
func (b *Bridge) UpdateCurve(c CurveConfig) (State, error) {
	if c.MinHR < 40 || c.MinHR > 220 || c.MaxHR <= c.MinHR || c.MaxHR > 240 ||
		c.MinSpeed < 0 || c.MinSpeed > 100 || c.MaxSpeed < c.MinSpeed || c.MaxSpeed > 100 ||
		c.WindowSeconds < 10 || c.WindowSeconds > 600 || c.Percentile < 1 || c.Percentile > 100 {
		return State{}, fmt.Errorf("invalid curve values (BPM 40-240 with min<max, Fan 0-100%%, Window 10-600s, P 1-100%%)")
	}

	b.mu.Lock()
	b.cfg.MinHR = c.MinHR
	b.cfg.MaxHR = c.MaxHR
	b.cfg.MinSpeed = c.MinSpeed
	b.cfg.MaxSpeed = c.MaxSpeed
	b.cfg.WindowSeconds = c.WindowSeconds
	b.cfg.Percentile = c.Percentile
	cfg := b.cfg
	cfgPath := b.cfgPath
	loop := b.loop
	ctx := b.ctx
	b.mu.Unlock()

	if cfgPath != "" {
		if err := config.Save(cfgPath, cfg); err != nil {
			return State{}, fmt.Errorf("save config: %w", err)
		}
	}

	if loop != nil {
		loop.SetMapper(ctx, control.LinearMapper{
			MinHR:    cfg.MinHR,
			MaxHR:    cfg.MaxHR,
			MinSpeed: cfg.MinSpeed,
			MaxSpeed: cfg.MaxSpeed,
		})
		loop.SetWindowConfig(ctx, cfg.WindowSeconds, cfg.Percentile)
	}

	b.Notify()
	return b.Snapshot(), nil
}

// SetHardware configures the BLE HR source, fan discoverer, and fan controller factory used by WebUI discovery.
func (b *Bridge) SetHardware(hrSource domain.HeartRateSource, fanDisc domain.FanDiscoverer, fanFactory FanFactory) {
	b.mu.Lock()
	b.hrSource = hrSource
	b.fanDisc = fanDisc
	b.fanFactory = fanFactory
	b.mu.Unlock()
}

// SetOnDeviceSync registers a callback invoked when a device is connected or selected via the WebUI.
func (b *Bridge) SetOnDeviceSync(fn func(hr *domain.HRDevice, fan *domain.FanDevice, cfg config.Config, fanCtrl domain.FanController)) {
	b.mu.Lock()
	b.onDeviceSync = fn
	b.mu.Unlock()
}

// CancelHRStream cancels any active HR reading stream started by Bridge.ConnectHR.
func (b *Bridge) CancelHRStream() {
	b.mu.Lock()
	cancel := b.hrStreamCancel
	b.hrStreamCancel = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StartScan starts a 15-second BLE HR and Shelly mDNS discovery scan and streams results via SSE.
func (b *Bridge) StartScan() (State, error) {
	b.mu.Lock()
	hrSource := b.hrSource
	fanDisc := b.fanDisc
	if hrSource == nil && fanDisc == nil {
		b.mu.Unlock()
		return State{}, fmt.Errorf("device discovery is not configured")
	}
	prevCancel := b.scanCancel
	b.scanGen++
	gen := b.scanGen
	scanCtx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	b.scanCancel = cancel
	b.discoveredHR = nil
	b.discoveredFan = nil
	b.bleErr = ""
	b.fanErr = ""
	b.bleScanning = hrSource != nil
	b.fanScanning = fanDisc != nil
	b.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
	}
	b.Notify()

	var bleCh <-chan domain.HRDevice
	var bleErr error
	if hrSource != nil {
		bleCh, bleErr = hrSource.Scan(scanCtx)
	}

	var fanCh <-chan domain.FanDevice
	var fanErr error
	if fanDisc != nil {
		fanCh, fanErr = fanDisc.Scan(scanCtx)
	}

	b.mu.Lock()
	if b.scanGen != gen {
		b.mu.Unlock()
		cancel()
		return b.Snapshot(), nil
	}
	if bleErr != nil {
		b.bleScanning = false
		b.bleErr = bleErr.Error()
	} else if bleCh == nil {
		b.bleScanning = false
	}
	if fanErr != nil {
		b.fanScanning = false
		b.fanErr = fanErr.Error()
	} else if fanCh == nil {
		b.fanScanning = false
	}
	b.mu.Unlock()
	b.Notify()

	if bleCh != nil {
		go b.consumeBLEScan(gen, bleCh)
	}
	if fanCh != nil {
		go b.consumeFanScan(gen, fanCh)
	}

	return b.Snapshot(), nil
}

func (b *Bridge) consumeBLEScan(gen uint64, ch <-chan domain.HRDevice) {
	for dev := range ch {
		b.mu.Lock()
		if b.scanGen != gen {
			b.mu.Unlock()
			return
		}
		found := false
		for i := range b.discoveredHR {
			if b.discoveredHR[i].ID == dev.ID {
				b.discoveredHR[i] = dev
				found = true
				break
			}
		}
		if !found {
			b.discoveredHR = append(b.discoveredHR, dev)
		}
		b.mu.Unlock()
		b.Notify()
	}
	b.mu.Lock()
	if b.scanGen == gen {
		b.bleScanning = false
	}
	b.mu.Unlock()
	b.Notify()
}

func (b *Bridge) consumeFanScan(gen uint64, ch <-chan domain.FanDevice) {
	for dev := range ch {
		b.mu.Lock()
		if b.scanGen != gen {
			b.mu.Unlock()
			return
		}
		found := false
		for i := range b.discoveredFan {
			if b.discoveredFan[i].ID == dev.ID {
				b.discoveredFan[i] = dev
				found = true
				break
			}
		}
		if !found {
			b.discoveredFan = append(b.discoveredFan, dev)
		}
		b.mu.Unlock()
		b.Notify()
	}
	b.mu.Lock()
	if b.scanGen == gen {
		b.fanScanning = false
	}
	b.mu.Unlock()
	b.Notify()
}

// StopScan cancels any active discovery scan and notifies subscribers.
func (b *Bridge) StopScan() State {
	b.mu.Lock()
	cancel := b.scanCancel
	b.scanCancel = nil
	b.scanGen++
	b.bleScanning = false
	b.fanScanning = false
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.Notify()
	return b.Snapshot()
}

// ConnectHR connects to a BLE heart rate monitor by ID, persists it, and streams readings into the control loop.
func (b *Bridge) ConnectHR(id string) (State, error) {
	if id == "" {
		return State{}, fmt.Errorf("device id is required")
	}

	b.mu.Lock()
	hrSource := b.hrSource
	if hrSource == nil {
		b.mu.Unlock()
		return State{}, fmt.Errorf("heart rate source is not configured")
	}
	dev := domain.HRDevice{ID: id, Name: id}
	for _, d := range b.discoveredHR {
		if d.ID == id {
			dev = d
			break
		}
	}
	if dev.Name == "" {
		dev.Name = dev.ID
	}
	scanCancel := b.scanCancel
	b.scanCancel = nil
	b.scanGen++
	b.bleScanning = false
	b.fanScanning = false
	prevHRCancel := b.hrStreamCancel
	b.hrStreamCancel = nil
	b.mu.Unlock()

	if scanCancel != nil {
		scanCancel()
	}
	if prevHRCancel != nil {
		prevHRCancel()
	}

	connCtx, cancel := context.WithCancel(b.ctx)
	ch, err := hrSource.Connect(connCtx, dev.ID)
	if err != nil {
		cancel()
		return State{}, fmt.Errorf("connect HR %s: %w", dev.ID, err)
	}

	b.mu.Lock()
	b.hrStreamCancel = cancel
	cp := dev
	b.hrDev = &cp
	b.cfg.BLEDeviceID = dev.ID
	b.cfg.BLEDeviceName = dev.Name
	b.errMsg = ""
	b.statusMsg = fmt.Sprintf("Connected HR: %s", dev.Name)
	cfg := b.cfg
	cfgPath := b.cfgPath
	syncFn := b.onDeviceSync
	b.mu.Unlock()

	if cfgPath != "" {
		if err := config.Save(cfgPath, cfg); err != nil {
			return State{}, fmt.Errorf("save config: %w", err)
		}
	}

	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case r, ok := <-ch:
				if !ok {
					b.mu.Lock()
					b.statusMsg = "HR stream closed"
					b.mu.Unlock()
					b.Notify()
					return
				}
				b.mu.RLock()
				loop := b.loop
				ctx := b.ctx
				b.mu.RUnlock()
				if loop != nil {
					loop.HandleHR(ctx, r)
				}
				b.Notify()
			}
		}
	}()

	if syncFn != nil {
		syncFn(&cp, nil, cfg, nil)
	}
	b.Notify()
	return b.Snapshot(), nil
}

// SelectFan selects a discovered Shelly dimmer by ID, updates the control loop, and persists it.
func (b *Bridge) SelectFan(id string) (State, error) {
	if id == "" {
		return State{}, fmt.Errorf("device id is required")
	}

	b.mu.Lock()
	var dev domain.FanDevice
	found := false
	for _, d := range b.discoveredFan {
		if d.ID == id {
			dev = d
			found = true
			break
		}
	}
	if !found && b.fanDev != nil && b.fanDev.ID == id {
		dev = *b.fanDev
		found = true
	}
	if !found {
		b.mu.Unlock()
		return State{}, fmt.Errorf("fan device %q not found in discovery results", id)
	}
	factory := b.fanFactory
	b.mu.Unlock()

	var fanCtrl domain.FanController
	if factory != nil {
		fanCtrl = factory(dev)
	}

	b.mu.Lock()
	cp := dev
	b.fanDev = &cp
	b.cfg.FanDeviceID = dev.ID
	b.cfg.FanDeviceName = dev.Name
	b.cfg.FanModel = dev.Model
	if dev.IP != "" {
		port := dev.Port
		if port == 0 {
			port = 80
		}
		b.cfg.FanHost = fmt.Sprintf("%s:%d", dev.IP, port)
	}
	name := dev.Name
	if name == "" {
		name = dev.ID
	}
	b.errMsg = ""
	b.statusMsg = fmt.Sprintf("Selected Shelly: %s", name)
	cfg := b.cfg
	cfgPath := b.cfgPath
	loop := b.loop
	ctx := b.ctx
	syncFn := b.onDeviceSync
	b.mu.Unlock()

	if cfgPath != "" {
		if err := config.Save(cfgPath, cfg); err != nil {
			return State{}, fmt.Errorf("save config: %w", err)
		}
	}

	if loop != nil {
		loop.SetFan(ctx, fanCtrl)
	}
	if syncFn != nil {
		syncFn(nil, &cp, cfg, fanCtrl)
	}
	b.Notify()
	return b.Snapshot(), nil
}
