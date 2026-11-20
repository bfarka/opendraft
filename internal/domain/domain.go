package domain

import (
	"context"
	"time"
)

// HRDevice represents a discovered Bluetooth Low Energy heart rate monitor.
type HRDevice struct {
	ID   string
	Name string
	RSSI int16
}

// HRReading represents a single heart rate measurement in beats per minute.
type HRReading struct {
	BPM       uint8
	Timestamp time.Time
}

// HeartRateSource discovers and streams live heart rate measurements.
type HeartRateSource interface {
	Scan(ctx context.Context) (<-chan HRDevice, error)
	Connect(ctx context.Context, deviceID string) (<-chan HRReading, error)
	Close() error
}

// FanDevice represents a discovered controllable fan / dimmer on the network.
type FanDevice struct {
	ID        string
	Name      string
	Host      string
	IP        string
	Port      int
	Encrypted bool
	Model     string
}

// FanDiscoverer scans for controllable fan devices on the local network.
type FanDiscoverer interface {
	Scan(ctx context.Context) (<-chan FanDevice, error)
}

// FanController adjusts the speed of a fan device (0 = off, 1..100 = percentage).
type FanController interface {
	SetSpeed(ctx context.Context, percent int) error
	Off(ctx context.Context) error
	Info() FanDevice
}

// SpeedMapper translates a heart rate (BPM) into a target fan speed percentage (0..100).
type SpeedMapper interface {
	Map(bpm uint8) int
}
