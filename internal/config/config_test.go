package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingReturnsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	def := Default()
	if !reflect.DeepEqual(cfg, def) {
		t.Fatalf("expected default config %+v, got %+v", def, cfg)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Config{
		BLEDeviceID:   "AA:BB:CC:DD:EE:FF",
		BLEDeviceName: "TICKR 1234",
		FanDeviceID:   "shellydimmer2-123456",
		FanDeviceName: "shellydimmer2-123456 (Shelly)",
		FanHost:       "192.168.1.80:80",
		FanModel:      "shelly",
		MinHR:         120,
		MaxHR:         165,
		MinSpeed:      15,
		MaxSpeed:      100,
		WindowSeconds: 90,
		Percentile:    85,
		WebPort:       9090,
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch: want %+v, got %+v", want, got)
	}
}
