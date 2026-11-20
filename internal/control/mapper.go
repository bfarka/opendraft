package control

// LinearMapper maps heart rate (BPM) linearly between MinHR..MaxHR to MinSpeed..MaxSpeed.
type LinearMapper struct {
	MinHR    uint8
	MaxHR    uint8
	MinSpeed int
	MaxSpeed int
}

// Map returns a fan speed percentage (0..100) for the given BPM.
// Any non-zero BPM up to MinHR returns fixed MinSpeed; above MinHR it ramps linearly to MaxSpeed at MaxHR.
func (m LinearMapper) Map(bpm uint8) int {
	if bpm == 0 {
		return 0
	}
	if bpm <= m.MinHR {
		return clampPercent(m.MinSpeed)
	}
	if m.MaxHR <= m.MinHR || bpm >= m.MaxHR {
		return clampPercent(m.MaxSpeed)
	}

	hrSpan := float64(m.MaxHR - m.MinHR)
	speedSpan := float64(m.MaxSpeed - m.MinSpeed)
	ratio := float64(bpm-m.MinHR) / hrSpan
	speed := float64(m.MinSpeed) + ratio*speedSpan
	return clampPercent(int(speed + 0.5))
}

func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
