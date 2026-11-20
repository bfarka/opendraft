package shelly

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"

	"github.com/bfarka/opendraft/internal/domain"
)

// Discoverer browses the local network via mDNS for Shelly devices (_http._tcp and _shelly._tcp).
type Discoverer struct{}

// NewDiscoverer creates a Shelly mDNS Discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Scan discovers Shelly devices on the LAN until ctx is canceled.
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
				select {
				case out <- dev:
				case <-ctx.Done():
					return
				}
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

func parseServiceEntry(entry *zeroconf.ServiceEntry) (domain.FanDevice, bool) {
	instLower := strings.ToLower(entry.Instance)
	hostLower := strings.ToLower(entry.HostName)
	if !strings.Contains(instLower, "shelly") && !strings.Contains(hostLower, "shelly") {
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
