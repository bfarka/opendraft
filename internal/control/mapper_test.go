package control

import "testing"

func TestLinearMapper(t *testing.T) {
	m := LinearMapper{
		MinHR:    120,
		MaxHR:    165,
		MinSpeed: 15,
		MaxSpeed: 100,
	}

	tests := []struct {
		name string
		bpm  uint8
		want int
	}{
		{name: "zero bpm", bpm: 0, want: 0},
		{name: "below entry HR uses fixed min speed", bpm: 95, want: 15},
		{name: "at entry HR", bpm: 120, want: 15},
		{name: "midpoint HR", bpm: 142, want: 57}, // 15 + (22/45)*85 = 56.55 -> 57
		{name: "at max HR", bpm: 165, want: 100},
		{name: "above max HR", bpm: 190, want: 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.Map(tc.bpm); got != tc.want {
				t.Fatalf("Map(%d) = %d, want %d", tc.bpm, got, tc.want)
			}
		})
	}
}
