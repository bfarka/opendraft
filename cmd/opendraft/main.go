package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bfarka/opendraft/internal/config"
	"github.com/bfarka/opendraft/internal/control"
	"github.com/bfarka/opendraft/internal/domain"
	"github.com/bfarka/opendraft/internal/fan/shelly"
	"github.com/bfarka/opendraft/internal/hr/ble"
	"github.com/bfarka/opendraft/internal/tui"
	"github.com/bfarka/opendraft/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "opendraft: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("opendraft", flag.ContinueOnError)
	cfgPathFlag := fs.String("config", "", "path to config JSON file")
	mockFlag := fs.Bool("mock", false, "use simulated BLE HR and Shelly dimmer devices (no hardware required)")
	webAddrFlag := fs.String("web-addr", ":8081", "HTTP address for local WebUI (empty to disable)")
	webPortFlag := fs.Int("web-port", 0, "TCP port for local WebUI (overrides config web_port; 0 disables)")
	headlessFlag := fs.Bool("headless", false, "run without TUI (serves WebUI and auto-connects saved/mock devices)")
	debugScanFlag := fs.Bool("debug-scan", false, "run a 5s diagnostic BLE + mDNS scan and print raw results")
	debugHRFlag := fs.Bool("debug-hr", false, "scan for first BLE HR monitor, connect, and print BPM for 10s")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var webAddrSet, webPortSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "web-addr":
			webAddrSet = true
		case "web-port":
			webPortSet = true
		}
	})

	if *debugScanFlag {
		return runDebugScan()
	}
	if *debugHRFlag {
		return runDebugHR()
	}

	cfgPath := *cfgPathFlag
	if cfgPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return err
		}
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	webAddr, err := resolveWebAddr(cfg.WebPort, *webAddrFlag, webAddrSet, *webPortFlag, webPortSet)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var hrSource domain.HeartRateSource
	var fanDisc domain.FanDiscoverer
	var fanFactory tui.FanFactory

	if *mockFlag {
		hrSource = &mockHRSource{}
		fanDisc = &mockFanDiscoverer{}
		fanFactory = func(dev domain.FanDevice) domain.FanController {
			return &mockFanController{dev: dev}
		}
	} else {
		bleSrc := ble.NewSource()
		defer bleSrc.Close()
		hrSource = bleSrc
		fanDisc = shelly.NewDiscoverer()
		fanFactory = func(dev domain.FanDevice) domain.FanController {
			return shelly.NewClient(dev)
		}
	}

	var bridge *web.Bridge
	var webURL string
	if webAddr != "" || *headlessFlag {
		bridge = web.NewBridge(ctx, nil, cfgPath, cfg)
		bridge.SetHardware(hrSource, fanDisc, func(dev domain.FanDevice) domain.FanController {
			return fanFactory(dev)
		})
		if webAddr != "" {
			boundAddr, _, err := web.StartServer(ctx, webAddr, bridge)
			if err != nil {
				if errors.Is(err, syscall.EADDRINUSE) {
					return fmt.Errorf("webui port already in use (%s): %w", webAddr, err)
				}
				return fmt.Errorf("start web server (%s): %w", webAddr, err)
			}
			webURL = formatWebURL(boundAddr)
			if *headlessFlag {
				fmt.Printf("opendraft WebUI listening on %s\n", webURL)
			}
		}
	}

	if *headlessFlag {
		return runHeadless(ctx, hrSource, fanDisc, fanFactory, cfg, *mockFlag, bridge)
	}

	m := tui.NewModel(ctx, hrSource, fanDisc, fanFactory, cfgPath, cfg).
		WithWebBridge(bridge).
		WithWebURL(webURL)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	if bridge != nil {
		bridge.SetOnDeviceSync(func(hr *domain.HRDevice, fan *domain.FanDevice, c config.Config, fanCtrl domain.FanController) {
			p.Send(tui.WebDeviceSyncMsg{
				HR:      hr,
				Fan:     fan,
				Cfg:     c,
				FanCtrl: fanCtrl,
			})
		})
	}
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}

func resolveWebAddr(cfgWebPort int, webAddr string, webAddrSet bool, webPort int, webPortSet bool) (string, error) {
	if webPortSet {
		if webPort < 0 || webPort > 65535 {
			return "", fmt.Errorf("invalid --web-port %d: must be 0..65535", webPort)
		}
		if webPort == 0 {
			return "", nil
		}
		if webAddrSet {
			if webAddr == "" {
				return "", nil
			}
			host, _, err := net.SplitHostPort(webAddr)
			if err != nil {
				return "", fmt.Errorf("invalid --web-addr %q: %w", webAddr, err)
			}
			return net.JoinHostPort(host, strconv.Itoa(webPort)), nil
		}
		return ":" + strconv.Itoa(webPort), nil
	}
	if webAddrSet {
		return webAddr, nil
	}
	if cfgWebPort < 0 || cfgWebPort > 65535 {
		return "", fmt.Errorf("invalid config web_port %d: must be 0..65535", cfgWebPort)
	}
	if cfgWebPort == 0 {
		return "", nil
	}
	return ":" + strconv.Itoa(cfgWebPort), nil
}

func formatWebURL(addr string) string {
	return formatWebURLWithIP(addr, localIPv4)
}

func formatWebURLWithIP(addr string, resolveIP func() string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if ip := resolveIP(); ip != "" {
			host = ip
		} else {
			host = "127.0.0.1"
		}
	}
	return "http://" + net.JoinHostPort(host, port)
}

func localIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			if ip4.IsPrivate() {
				return ip4.String()
			}
			if fallback == "" {
				fallback = ip4.String()
			}
		}
	}
	return fallback
}

func runHeadless(
	ctx context.Context,
	hrSource domain.HeartRateSource,
	fanDisc domain.FanDiscoverer,
	fanFactory tui.FanFactory,
	cfg config.Config,
	useMock bool,
	bridge *web.Bridge,
) error {
	mapper := control.LinearMapper{
		MinHR:    cfg.MinHR,
		MaxHR:    cfg.MaxHR,
		MinSpeed: cfg.MinSpeed,
		MaxSpeed: cfg.MaxSpeed,
	}

	var fanDev *domain.FanDevice
	var fanCtrl domain.FanController
	if cfg.HasSavedDevices() {
		d := cfg.SavedFanDevice()
		fanDev = &d
		fanCtrl = fanFactory(d)
	} else if useMock && fanDisc != nil {
		scanCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ch, err := fanDisc.Scan(scanCtx)
		if err == nil {
			if d, ok := <-ch; ok {
				fanDev = &d
				fanCtrl = fanFactory(d)
			}
		}
		cancel()
	}

	loop := control.NewLoop(mapper, fanCtrl)
	loop.SetWindowConfig(ctx, cfg.WindowSeconds, cfg.Percentile)
	bridge.SetLoop(loop)
	bridge.SetDevices(nil, fanDev)
	loop.Start(ctx)
	bridge.Notify()

	var hrCh <-chan domain.HRReading
	targetHRID := cfg.BLEDeviceID
	if useMock && targetHRID == "" {
		targetHRID = "mock-tickr"
	}
	if targetHRID != "" && hrSource != nil {
		hrDev := &domain.HRDevice{ID: targetHRID, Name: cfg.BLEDeviceName}
		if hrDev.Name == "" {
			hrDev.Name = targetHRID
		}
		ch, err := hrSource.Connect(ctx, targetHRID)
		if err != nil {
			bridge.SetStatus("", fmt.Sprintf("HR connect: %v", err))
		} else {
			hrCh = ch
			bridge.SetDevices(hrDev, fanDev)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			loop.Stop(context.Background())
			if fanCtrl != nil {
				_ = fanCtrl.Off(context.Background())
			}
			return nil
		case r, ok := <-hrCh:
			if !ok {
				hrCh = nil
				bridge.SetStatus("HR stream closed", "")
				continue
			}
			loop.HandleHR(ctx, r)
			bridge.Notify()
		case <-ticker.C:
			loop.CheckTimeout(ctx)
			bridge.Notify()
		}
	}
}

// Mock adapters for --mock mode.

type mockHRSource struct{}

func (m *mockHRSource) Scan(ctx context.Context) (<-chan domain.HRDevice, error) {
	ch := make(chan domain.HRDevice, 1)
	go func() {
		defer close(ch)
		select {
		case ch <- domain.HRDevice{ID: "mock-tickr", Name: "TICKR Mock 42", RSSI: -52}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

func (m *mockHRSource) Connect(ctx context.Context, _ string) (<-chan domain.HRReading, error) {
	ch := make(chan domain.HRReading, 8)
	go func() {
		defer close(ch)
		bpm := uint8(105)
		dir := int8(3)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				ch <- domain.HRReading{BPM: bpm, Timestamp: now}
				next := int(bpm) + int(dir)
				if next >= 175 {
					dir = -3
				} else if next <= 95 {
					dir = 3
				}
				bpm = uint8(next)
			}
		}
	}()
	return ch, nil
}

func (m *mockHRSource) Close() error { return nil }

type mockFanDiscoverer struct{}

func (m *mockFanDiscoverer) Scan(ctx context.Context) (<-chan domain.FanDevice, error) {
	ch := make(chan domain.FanDevice, 1)
	go func() {
		defer close(ch)
		select {
		case ch <- domain.FanDevice{
			ID:    "shellydimmer2-mock01",
			Name:  "shellydimmer2-mock01 (Shelly)",
			IP:    "192.168.1.99",
			Port:  80,
			Model: "shelly",
		}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

type mockFanController struct {
	dev domain.FanDevice
}

func (m *mockFanController) SetSpeed(_ context.Context, _ int) error { return nil }
func (m *mockFanController) Off(_ context.Context) error             { return nil }
func (m *mockFanController) Info() domain.FanDevice                  { return m.dev }
