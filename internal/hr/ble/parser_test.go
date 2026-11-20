package ble

import (
	"errors"
	"testing"
)

func TestParseHeartRateMeasurement(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    uint8
		wantErr bool
	}{
		{
			name: "8-bit value",
			data: []byte{0x00, 142},
			want: 142,
		},
		{
			name: "8-bit value with sensor contact and RR intervals",
			data: []byte{0x16, 165, 0x00, 0x01, 0x02, 0x03},
			want: 165,
		},
		{
			name: "16-bit value within uint8 range",
			data: []byte{0x01, 180, 0x00},
			want: 180,
		},
		{
			name: "16-bit value clamped to 255",
			data: []byte{0x01, 0x2C, 0x01}, // 300 bpm
			want: 255,
		},
		{
			name:    "empty payload",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "too short for 8-bit",
			data:    []byte{0x00},
			wantErr: true,
		},
		{
			name:    "too short for 16-bit",
			data:    []byte{0x01, 0x50},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHeartRateMeasurement(tc.data)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidPacket) {
					t.Fatalf("expected ErrInvalidPacket, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}
