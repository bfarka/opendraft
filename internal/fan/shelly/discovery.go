package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"

	"github.com/bfarka/opendraft/internal/domain"
)

const probeTimeout = 1500 * time.Millisecond

// Discoverer browses the local network via mDNS for Shelly dimmer devices (_http._tcp and _shelly._tcp).
type Discoverer struct {
	httpClient *http.Client
}

// NewDiscoverer creates a Shelly mDNS Discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{
		httpClient: &http.Client{Timeout: probeTimeout},
	}
}

// Scan discovers Shelly dimmer devices on the LAN until ctx is canceled.
func (d *Discoverer) Scan(ctx context.Context) (<-chan domain.FanDevice, error) {
	out := make(chan domain.FanDevice, 16)

	var seenMu sync.Mutex
	seen := make(map[string]struct{})

	var wg sync.WaitGroup
	browseService := func(service string) {
		defer wg.Done()
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			return
		}
		entries := make(chan *zeroconf.ServiceEntry, 16)
		go func() {
			_ = resolver.Browse(ctx, service, "local.", entries)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}
				dev, isShelly := parseServiceEntry(entry)
				if !isShelly {
					continue
				}
				seenMu.Lock()
				_, exists := seen[dev.ID]
				if !exists {
					seen[dev.ID] = struct{}{}
				}
				seenMu.Unlock()
				if exists {
					continue
				}

				wg.Add(1)
				go func(e *zeroconf.ServiceEntry, candidate domain.FanDevice) {
					defer wg.Done()
					verified, ok := d.verifyDimmer(ctx, e, candidate)
					if !ok {
						return
					}
					select {
					case out <- verified:
					case <-ctx.Done():
					}
				}(entry, dev)
			}
		}
	}

	wg.Add(2)
	go browseService("_http._tcp")
	go browseService("_shelly._tcp")

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func (d *Discoverer) verifyDimmer(ctx context.Context, entry *zeroconf.ServiceEntry, dev domain.FanDevice) (domain.FanDevice, bool) {
	txt := parseTXT(entry.Text)
	if app, ok := txt["app"]; ok && app != "" && !isDimmerApp(app) {
		return domain.FanDevice{}, false
	}

	host := dev.IP
	if host == "" {
		host = strings.TrimSuffix(dev.Host, ".")
	}
	if host != "" && d.httpClient != nil {
		info, err := d.fetchShellyInfo(ctx, host, dev.Port)
		if err == nil {
			if info.Auth || info.AuthEn {
				dev.Encrypted = true
			}
			if known, dimmer := isDimmerInfo(info); known {
				if !dimmer {
					return domain.FanDevice{}, false
				}
				return dev, true
			}
		}
	}

	if !isDimmerEntry(entry) {
		return domain.FanDevice{}, false
	}
	return dev, true
}

type shellyInfo struct {
	Type   string `json:"type"`
	Model  string `json:"model"`
	App    string `json:"app"`
	Gen    int    `json:"gen"`
	Auth   bool   `json:"auth"`
	AuthEn bool   `json:"auth_en"`
}

func (d *Discoverer) fetchShellyInfo(ctx context.Context, host string, port int) (shellyInfo, error) {
	if port == 0 {
		port = 80
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+addr+"/shelly", nil)
	if err != nil {
		return shellyInfo{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return shellyInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return shellyInfo{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return shellyInfo{}, err
	}
	var info shellyInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return shellyInfo{}, err
	}
	return info, nil
}

func parseServiceEntry(entry *zeroconf.ServiceEntry) (domain.FanDevice, bool) {
	instLower := strings.ToLower(entry.Instance)
	hostLower := strings.ToLower(entry.HostName)
	txt := parseTXT(entry.Text)
	idLower := strings.ToLower(txt["id"])
	isShelly := strings.Contains(instLower, "shelly") ||
		strings.Contains(hostLower, "shelly") ||
		strings.Contains(idLower, "shelly") ||
		(txt["gen"] != "" && txt["app"] != "")
	if !isShelly {
		return domain.FanDevice{}, false
	}

	ip := ""
	if len(entry.AddrIPv4) > 0 {
		ip = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		ip = entry.AddrIPv6[0].String()
	}

	port := entry.Port
	if port == 0 {
		port = 80
	}

	return domain.FanDevice{
		ID:        entry.Instance,
		Name:      fmt.Sprintf("%s (Shelly)", entry.Instance),
		Host:      entry.HostName,
		IP:        ip,
		Port:      port,
		Encrypted: false,
		Model:     "shelly",
	}, true
}

func parseTXT(records []string) map[string]string {
	out := make(map[string]string, len(records))
	for _, r := range records {
		k, v, ok := strings.Cut(r, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

func isDimmerInfo(info shellyInfo) (known bool, dimmer bool) {
	if info.Type != "" {
		t := strings.ToUpper(info.Type)
		return true, t == "SHDM-1" || t == "SHDM-2"
	}
	if info.App != "" {
		return true, isDimmerApp(info.App)
	}
	if info.Model != "" {
		m := strings.ToUpper(info.Model)
		if strings.HasPrefix(m, "SNDM-") ||
			strings.HasPrefix(m, "SNWD-") ||
			strings.HasPrefix(m, "S3DM-") ||
			strings.HasPrefix(m, "SPDM-") {
			return true, true
		}
	}
	return false, false
}

func isDimmerEntry(entry *zeroconf.ServiceEntry) bool {
	txt := parseTXT(entry.Text)
	if app, ok := txt["app"]; ok && app != "" {
		return isDimmerApp(app)
	}
	return isDimmerName(entry.Instance) || isDimmerName(entry.HostName) || isDimmerName(txt["id"])
}

func isDimmerApp(app string) bool {
	a := strings.ToLower(app)
	return strings.Contains(a, "dimmer") || strings.Contains(a, "010v") || strings.Contains(a, "0110dim")
}

func isDimmerName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "dimmer") ||
		strings.Contains(n, "010v") ||
		strings.Contains(n, "0110dim") ||
		strings.Contains(n, "shellypluswd")
}
