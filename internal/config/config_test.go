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

func TestSavedHRDeviceFallback(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantID   string
		wantName string
		wantSave bool
	}{
		{
			name: "new hr fields take precedence",
			cfg: Config{
				HRDeviceID:    "ant:12345",
				HRDeviceName:  "ANT+ HR 12345",
				BLEDeviceID:   "AA:BB:CC:DD:EE:FF",
				BLEDeviceName: "Legacy TICKR",
				FanDeviceID:   "fan-1",
				FanHost:       "192.168.1.10:80",
			},
			wantID:   "ant:12345",
			wantName: "ANT+ HR 12345",
			wantSave: true,
		},
		{
			name: "legacy ble fields fallback",
			cfg: Config{
				BLEDeviceID:   "AA:BB:CC:DD:EE:FF",
				BLEDeviceName: "Legacy TICKR",
				FanDeviceID:   "fan-1",
				FanHost:       "192.168.1.10:80",
			},
			wantID:   "AA:BB:CC:DD:EE:FF",
			wantName: "Legacy TICKR",
			wantSave: true,
		},
		{
			name: "missing hr device",
			cfg: Config{
				FanDeviceID: "fan-1",
				FanHost:     "192.168.1.10:80",
			},
			wantID:   "",
			wantName: "",
			wantSave: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.SavedHRDeviceID(); got != tc.wantID {
				t.Errorf("SavedHRDeviceID() = %q, want %q", got, tc.wantID)
			}
			if got := tc.cfg.SavedHRDeviceName(); got != tc.wantName {
				t.Errorf("SavedHRDeviceName() = %q, want %q", got, tc.wantName)
			}
			if got := tc.cfg.HasSavedDevices(); got != tc.wantSave {
				t.Errorf("HasSavedDevices() = %v, want %v", got, tc.wantSave)
			}
		})
	}
}
