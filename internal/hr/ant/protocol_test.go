package ant

import (
	"bytes"
	"testing"
)

func TestEncodeFrame(t *testing.T) {
	got := EncodeFrame(msgResetSystem, 0x00)
	want := []byte{0xA4, 0x01, 0x4A, 0x00, 0xEF}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFrame() = % X, want % X", got, want)
	}
}

func TestFrameDecoder(t *testing.T) {
	f1 := EncodeFrame(msgStartupMessage, 0x00)
	f2 := EncodeFrame(msgBroadcastData, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 142)

	t.Run("concatenated and partial reads", func(t *testing.T) {
		var dec FrameDecoder
		stream := append(append([]byte{0xFF, 0x00}, f1...), f2...)

		got1 := dec.Feed(stream[:8])
		if len(got1) != 1 || got1[0].MsgID != msgStartupMessage {
			t.Fatalf("expected 1 startup frame from first chunk, got %+v", got1)
		}

		got2 := dec.Feed(stream[8:])
		if len(got2) != 1 || got2[0].MsgID != msgBroadcastData || got2[0].Payload[8] != 142 {
			t.Fatalf("expected 1 broadcast frame with BPM 142, got %+v", got2)
		}
	})

	t.Run("corrupted checksum resyncs", func(t *testing.T) {
		var dec FrameDecoder
		bad := append([]byte(nil), f1...)
		bad[len(bad)-1] ^= 0xFF
		stream := append(bad, f2...)

		got := dec.Feed(stream)
		if len(got) != 1 || got[0].MsgID != msgBroadcastData {
			t.Fatalf("expected resync to valid frame, got %+v", got)
		}
	})
}

func TestParseHRBroadcast(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    HRBroadcast
		wantErr bool
	}{
		{
			name:    "standard 9-byte broadcast",
			payload: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x05, 138},
			want: HRBroadcast{
				Channel: 0,
				BPM:     138,
			},
		},
		{
			name: "extended broadcast with channel ID and RSSI",
			payload: []byte{
				0x00,
				0x00, 0xFF, 0xFF, 0xFF, 0x10, 0x20, 0x0A, 155,
				0xC0,       // Channel ID (0x80) | RSSI (0x40)
				0x39, 0x30, // DeviceNumber = 0x3039 = 12345
				0x78, // DeviceType = 120 (HRM)
				0x01, // TransType = 1
				0x10, // RSSI measurement type
				0xC4, // -60 dBm (int8)
				0x00, // Threshold
			},
			want: HRBroadcast{
				Channel:      0,
				BPM:          155,
				HasExtID:     true,
				DeviceNumber: 12345,
				DeviceType:   120,
				TransType:    1,
				HasRSSI:      true,
				RSSI:         -60,
			},
		},
		{
			name:    "too short",
			payload: []byte{0x00, 0x01, 0x02},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHRBroadcast(tc.payload)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseHRBroadcast() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
