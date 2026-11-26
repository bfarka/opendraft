package multi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/bfarka/opendraft/internal/domain"
)

// NamedSource pairs a transport identifier (e.g. "ble", "ant") with a HeartRateSource.
type NamedSource struct {
	Name   string
	Source domain.HeartRateSource
}

// Source combines multiple HeartRateSource implementations behind a single HeartRateSource.
// Discovered device IDs are prefixed with "<transport>:" (e.g. "ble:AA:BB:...", "ant:12345").
type Source struct {
	sources []NamedSource
}

// New creates a composite HeartRateSource from the given named sources.
func New(sources ...NamedSource) *Source {
	var valid []NamedSource
	for _, s := range sources {
		if s.Source != nil && s.Name != "" {
			valid = append(valid, s)
		}
	}
	return &Source{sources: valid}
}

// Scan runs Scan on all configured sources concurrently, merging discovered HR devices.
// If at least one source starts scanning, errors from other unavailable sources (e.g., no ANT+ stick)
// are ignored. Only if all sources fail does Scan return an error.
func (s *Source) Scan(ctx context.Context) (<-chan domain.HRDevice, error) {
	if len(s.sources) == 0 {
		return nil, errors.New("multi: no heart rate sources configured")
	}

	type activeScan struct {
		name string
		ch   <-chan domain.HRDevice
	}

	var active []activeScan
	var errs []error

	for _, ns := range s.sources {
		ch, err := ns.Source.Scan(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ns.Name, err))
			continue
		}
		active = append(active, activeScan{name: ns.Name, ch: ch})
	}

	if len(active) == 0 {
		return nil, errors.Join(errs...)
	}

	out := make(chan domain.HRDevice, 32)
	var wg sync.WaitGroup
	for _, a := range active {
		wg.Add(1)
		go func(name string, ch <-chan domain.HRDevice) {
			defer wg.Done()
			for dev := range ch {
				if dev.Transport == "" {
					dev.Transport = name
				}
				prefix := name + ":"
				if !strings.HasPrefix(dev.ID, prefix) {
					dev.ID = prefix + dev.ID
				}
				select {
				case out <- dev:
				case <-ctx.Done():
					return
				}
			}
		}(a.name, a.ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// Connect routes a prefixed deviceID ("ble:...", "ant:...") or legacy unprefixed BLE ID
// to the corresponding underlying HeartRateSource.
func (s *Source) Connect(ctx context.Context, deviceID string) (<-chan domain.HRReading, error) {
	if len(s.sources) == 0 {
		return nil, errors.New("multi: no heart rate sources configured")
	}

	for _, ns := range s.sources {
		prefix := ns.Name + ":"
		if strings.HasPrefix(deviceID, prefix) {
			return ns.Source.Connect(ctx, strings.TrimPrefix(deviceID, prefix))
		}
	}

	// Legacy unprefixed ID (including MAC addresses like "11:22:33:44:55:66"):
	// default to BLE if available, otherwise first source.
	for _, ns := range s.sources {
		if ns.Name == "ble" {
			return ns.Source.Connect(ctx, deviceID)
		}
	}
	return s.sources[0].Source.Connect(ctx, deviceID)
}

// Close closes all underlying HeartRateSource implementations.
func (s *Source) Close() error {
	var errs []error
	for _, ns := range s.sources {
		if err := ns.Source.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ns.Name, err))
		}
	}
	return errors.Join(errs...)
}

// MatchDeviceID reports whether a saved config ID matches a discovered device ID,
// supporting both prefixed ("ble:AA:BB") and legacy unprefixed ("AA:BB") saved IDs.
func MatchDeviceID(savedID, discoveredID string) bool {
	if savedID == "" || discoveredID == "" {
		return false
	}
	if savedID == discoveredID {
		return true
	}
	if !strings.Contains(savedID, ":") || strings.Count(savedID, ":") >= 2 {
		// Legacy BLE MAC/UUID without "ble:" prefix
		return discoveredID == "ble:"+savedID
	}
	return false
}
