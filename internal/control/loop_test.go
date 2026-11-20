package control

import (
	"context"
	"testing"
	"time"

	"github.com/bfarka/opendraft/internal/domain"
)

type fakeFan struct {
	speeds []int
	offs   int
}

func (f *fakeFan) SetSpeed(_ context.Context, percent int) error {
	f.speeds = append(f.speeds, percent)
	return nil
}

func (f *fakeFan) Off(_ context.Context) error {
	f.offs++
	f.speeds = append(f.speeds, 0)
	return nil
}

func (f *fakeFan) Info() domain.FanDevice {
	return domain.FanDevice{ID: "fake", Name: "Fake Fan"}
}

func TestLoopStartStopAutoRateLimitingManualAndTimeout(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	fan := &fakeFan{}
	mapper := LinearMapper{MinHR: 120, MaxHR: 165, MinSpeed: 15, MaxSpeed: 100}
	loop := NewLoop(mapper, fan)
	loop.nowFn = func() time.Time { return now }

	ctx := context.Background()

	// While stopped, HR updates BPM & TargetSpeed but does NOT drive the fan.
	snap := loop.HandleHR(ctx, domain.HRReading{BPM: 110, Timestamp: now})
	if snap.Running || snap.BPM != 110 || snap.TargetSpeed != 15 || snap.AppliedSpeed != 0 || len(fan.speeds) != 0 {
		t.Fatalf("expected stopped loop not to drive fan, got %+v (calls=%v)", snap, fan.speeds)
	}

	// Start applies current target speed immediately (15% for 110 bpm <= 120).
	snap = loop.Start(ctx)
	if !snap.Running || snap.AppliedSpeed != 15 || len(fan.speeds) != 1 {
		t.Fatalf("expected Start() to apply 15%%, got %+v (calls=%v)", snap, fan.speeds)
	}

	// Within minInterval, quick change is rate-limited.
	now = now.Add(100 * time.Millisecond)
	snap = loop.HandleHR(ctx, domain.HRReading{BPM: 140, Timestamp: now})
	if snap.AppliedSpeed != 15 || len(fan.speeds) != 1 {
		t.Fatalf("expected rate-limit to hold 15, got %+v (calls=%v)", snap, fan.speeds)
	}

	// After minInterval, 165 bpm -> 100%.
	now = now.Add(600 * time.Millisecond)
	snap = loop.HandleHR(ctx, domain.HRReading{BPM: 165, Timestamp: now})
	if snap.AppliedSpeed != 100 || len(fan.speeds) != 2 {
		t.Fatalf("expected updated speed 100, got %+v (calls=%v)", snap, fan.speeds)
	}

	// Manual override switches mode and applies immediately.
	snap = loop.SetManualSpeed(ctx, 35)
	if snap.Mode != ModeManual || snap.AppliedSpeed != 35 {
		t.Fatalf("expected MANUAL 35, got %+v", snap)
	}

	// HR updates in MANUAL mode do not overwrite manual speed.
	now = now.Add(1 * time.Second)
	snap = loop.HandleHR(ctx, domain.HRReading{BPM: 165, Timestamp: now})
	if snap.AppliedSpeed != 35 {
		t.Fatalf("expected MANUAL mode to keep 35, got %+v", snap)
	}

	// Switch back to AUTO applies current HR speed immediately.
	snap = loop.SetMode(ctx, ModeAuto)
	if snap.Mode != ModeAuto || snap.AppliedSpeed != 100 {
		t.Fatalf("expected AUTO 100, got %+v", snap)
	}

	// Stop turns fan off.
	snap = loop.Stop(ctx)
	if snap.Running || snap.AppliedSpeed != 0 || fan.offs == 0 {
		t.Fatalf("expected Stop() to turn fan off, got %+v (offs=%d)", snap, fan.offs)
	}

	// Start again and verify HR timeout turns fan off.
	snap = loop.Start(ctx)
	now = now.Add(15 * time.Second)
	snap = loop.CheckTimeout(ctx)
	if snap.HRConnected || snap.AppliedSpeed != 0 {
		t.Fatalf("expected HR timeout to turn fan off, got %+v", snap)
	}
}

func TestLoopStartAlwaysTurnsSwitchOnAndStopTurnsOff(t *testing.T) {
	fan := &fakeFan{}
	mapper := LinearMapper{MinHR: 120, MaxHR: 165, MinSpeed: 15, MaxSpeed: 100}
	loop := NewLoop(mapper, fan)
	ctx := context.Background()

	// Start before any HR reading arrives -> turns Shelly ON at MinSpeed (15%).
	snap := loop.Start(ctx)
	if !snap.Running || snap.AppliedSpeed != 15 || len(fan.speeds) != 1 || fan.speeds[0] != 15 {
		t.Fatalf("expected Start() to turn fan ON at 15%%, got %+v (calls=%v)", snap, fan.speeds)
	}

	// Stop -> turns Shelly OFF (0%).
	snap = loop.Stop(ctx)
	if snap.Running || snap.AppliedSpeed != 0 || fan.offs != 1 {
		t.Fatalf("expected Stop() to turn fan OFF, got %+v (offs=%d)", snap, fan.offs)
	}
}

func TestLoopUsesPercentileWindowAndClearsOnTimeout(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	fan := &fakeFan{}
	mapper := LinearMapper{MinHR: 120, MaxHR: 160, MinSpeed: 20, MaxSpeed: 100}
	loop := NewLoop(mapper, fan)
	loop.nowFn = func() time.Time { return now }
	ctx := context.Background()

	loop.SetWindowConfig(ctx, 10, 80)
	loop.Start(ctx)

	// Feed 10 readings: 8x 120 bpm, 2x 160 bpm.
	// P80 of 10 samples = rank 8 = 120 bpm -> 20%.
	for i := 0; i < 8; i++ {
		now = now.Add(time.Second)
		loop.HandleHR(ctx, domain.HRReading{BPM: 120, Timestamp: now})
	}
	for i := 0; i < 2; i++ {
		now = now.Add(time.Second)
		loop.HandleHR(ctx, domain.HRReading{BPM: 160, Timestamp: now})
	}
	snap := loop.State()
	if snap.BPM != 160 || snap.EffectiveBPM != 120 || snap.TargetSpeed != 20 {
		t.Fatalf("expected live=160, p80=120, target=20%%; got %+v", snap)
	}

	// Add one more 160 bpm (evicts a 120 -> 7x 120, 3x 160 -> rank 8 = 160 bpm -> 100%).
	now = now.Add(time.Second)
	snap = loop.HandleHR(ctx, domain.HRReading{BPM: 160, Timestamp: now})
	if snap.EffectiveBPM != 160 || snap.TargetSpeed != 100 {
		t.Fatalf("expected p80=160, target=100%%; got %+v", snap)
	}

	// Timeout clears the window.
	now = now.Add(15 * time.Second)
	snap = loop.CheckTimeout(ctx)
	if snap.WindowCount != 0 || snap.EffectiveBPM != 0 || snap.AppliedSpeed != 0 {
		t.Fatalf("expected cleared window after timeout, got %+v", snap)
	}
}
