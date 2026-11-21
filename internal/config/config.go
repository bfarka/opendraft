package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bfarka/opendraft/internal/domain"
)

// Config holds persisted device selections and HR-to-fan curve settings.
type Config struct {
	BLEDeviceID   string `json:"ble_device_id,omitempty"`
	BLEDeviceName string `json:"ble_device_name,omitempty"`
	FanDeviceID   string `json:"fan_device_id,omitempty"`
	FanDeviceName string `json:"fan_device_name,omitempty"`
	FanHost       string `json:"fan_host,omitempty"`
	FanModel      string `json:"fan_model,omitempty"`
	MinHR         uint8  `json:"min_hr"`
	MaxHR         uint8  `json:"max_hr"`
	MinSpeed      int    `json:"min_speed"`
	MaxSpeed      int    `json:"max_speed"`
	WindowSeconds int    `json:"window_seconds"`
	Percentile    int    `json:"percentile"`
	WebPort       int    `json:"web_port"`
}

// HasSavedDevices reports whether both a BLE HR device and a fan dimmer are saved.
func (c Config) HasSavedDevices() bool {
	return c.BLEDeviceID != "" && c.FanDeviceID != "" && c.FanHost != ""
}

// SavedFanDevice reconstructs a domain.FanDevice from the persisted config fields.
func (c Config) SavedFanDevice() domain.FanDevice {
	host, portStr, err := net.SplitHostPort(c.FanHost)
	port := 80
	if err != nil {
		host = c.FanHost
	} else if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
		port = p
	}
	name := c.FanDeviceName
	if name == "" {
		name = c.FanDeviceID
	}
	return domain.FanDevice{
		ID:    c.FanDeviceID,
		Name:  name,
		IP:    host,
		Port:  port,
		Model: c.FanModel,
	}
}

// Default returns a Config populated with sensible indoor cycling defaults.
func Default() Config {
	return Config{
		MinHR:         120,
		MaxHR:         165,
		MinSpeed:      15,
		MaxSpeed:      100,
		WindowSeconds: 120,
		Percentile:    80,
		WebPort:       8081,
	}
}

// DefaultPath returns the default configuration file path (~/.config/opendraft/config.json).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "opendraft", "config.json"), nil
}

// Load reads Config from path. If the file does not exist, it returns Default() and nil error.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %q: %w", path, err)
	}
	def := Default()
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = def.WindowSeconds
	}
	if cfg.Percentile <= 0 || cfg.Percentile > 100 {
		cfg.Percentile = def.Percentile
	}
	return cfg, nil
}

// Save writes cfg as indented JSON to path, creating parent directories if needed.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir %q: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return nil
}
