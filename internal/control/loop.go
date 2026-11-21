package control

import (
	"context"
	"sync"
	"time"

	"github.com/bfarka/opendraft/internal/domain"
)

// Mode represents whether fan speed is driven by HR or set manually.
type Mode string

const (
	ModeAuto   Mode = "AUTO"
	ModeManual Mode = "MANUAL"
)

// Snapshot is a point-in-time view of the control loop state.
type Snapshot struct {
	Running       bool
	BPM           uint8
	EffectiveBPM  uint8
	WindowCount   int
	WindowSeconds int
	Percentile    int
	LastHRAt      time.Time
	TargetSpeed   int
	AppliedSpeed  int
	Mode          Mode
	HRConnected   bool
	LastErr       string
}

// Loop coordinates HR readings, speed mapping, rate-limiting, manual overrides, and disconnect safety.
type Loop struct {
	mapper      domain.SpeedMapper
	fan         domain.FanController
	minInterval time.Duration
	minDelta    int
	hrTimeout   time.Duration
	nowFn       func() time.Time

	mu           sync.Mutex
	running      bool
	mode         Mode
	bpm          uint8
	effectiveBPM uint8
	window       *HRWindow
	percentile   int
	lastHRAt     time.Time
	hrConnected  bool
	manualSpeed  int
	targetSpeed  int
	appliedSpeed int
	lastSentAt   time.Time
	hasSent      bool
	lastErr      string
}

// NewLoop creates a Loop with sensible rate-limiting and timeout defaults.
func NewLoop(mapper domain.SpeedMapper, fan domain.FanController) *Loop {
	return &Loop{
		mapper:      mapper,
		fan:         fan,
		minInterval: 500 * time.Millisecond,
		minDelta:    2,
		hrTimeout:   10 * time.Second,
		nowFn:       time.Now,
		mode:        ModeAuto,
		window:      NewHRWindow(120),
		percentile:  80,
	}
}

// State returns a thread-safe copy of the current control loop snapshot.
func (l *Loop) State() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snapshotLocked()
}

// Start enables fan control and immediately turns the fan on.
func (l *Loop) Start(ctx context.Context) Snapshot {
	l.mu.Lock()
	l.running = true
	if l.mode == ModeAuto {
		eff := l.effectiveHRLocked()
		if l.hrConnected && eff > 0 {
			l.targetSpeed = clampPercent(l.mapper.Map(eff))
		} else if l.mapper != nil {
			l.targetSpeed = clampPercent(l.mapper.Map(1))
		}
	} else {
		if l.manualSpeed == 0 && l.mapper != nil {
			l.manualSpeed = clampPercent(l.mapper.Map(1))
		}
		l.targetSpeed = l.manualSpeed
	}
	l.maybeApplyLocked(ctx, true)
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// Stop pauses fan control and turns the fan off.
func (l *Loop) Stop(ctx context.Context) Snapshot {
	l.mu.Lock()
	l.running = false
	l.targetSpeed = 0
	l.maybeApplyLocked(ctx, true)
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// SetMapper updates the active SpeedMapper and re-evaluates speed if running in AUTO mode.
func (l *Loop) SetMapper(ctx context.Context, mapper domain.SpeedMapper) Snapshot {
	l.mu.Lock()
	l.mapper = mapper
	if l.running && l.mode == ModeAuto && l.hrConnected {
		l.targetSpeed = clampPercent(l.mapper.Map(l.effectiveHRLocked()))
		l.maybeApplyLocked(ctx, true)
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// SetFan updates the active FanController and re-applies the current speed if running.
func (l *Loop) SetFan(ctx context.Context, fan domain.FanController) Snapshot {
	l.mu.Lock()
	l.fan = fan
	if l.running {
		l.maybeApplyLocked(ctx, true)
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// SetWindowConfig updates the rolling window duration and percentile and re-evaluates speed in AUTO mode.
func (l *Loop) SetWindowConfig(ctx context.Context, windowSeconds, percentile int) Snapshot {
	l.mu.Lock()
	if percentile <= 0 {
		percentile = 80
	} else if percentile > 100 {
		percentile = 100
	}
	l.window.Resize(windowSeconds)
	l.percentile = percentile
	l.effectiveBPM = l.window.Percentile(l.percentile)
	if l.mode == ModeAuto && l.hrConnected {
		l.targetSpeed = clampPercent(l.mapper.Map(l.effectiveHRLocked()))
		if l.running {
			l.maybeApplyLocked(ctx, true)
		}
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// History appends the rolling HR window samples (oldest to newest) to dst and returns it.
func (l *Loop) History(dst []uint8) []uint8 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.window.History(dst)
}

// HandleHR processes a new heart rate reading and updates the fan when running in AUTO mode.
func (l *Loop) HandleHR(ctx context.Context, r domain.HRReading) Snapshot {
	l.mu.Lock()
	ts := r.Timestamp
	if ts.IsZero() {
		ts = l.nowFn()
	}
	l.bpm = r.BPM
	l.lastHRAt = ts
	l.hrConnected = true
	l.window.Add(r.BPM, ts)
	l.effectiveBPM = l.window.Percentile(l.percentile)
	if l.mode == ModeAuto {
		l.targetSpeed = clampPercent(l.mapper.Map(l.effectiveHRLocked()))
		if l.running {
			l.maybeApplyLocked(ctx, false)
		}
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// SetMode switches between ModeAuto and ModeManual.
func (l *Loop) SetMode(ctx context.Context, mode Mode) Snapshot {
	l.mu.Lock()
	l.mode = mode
	if mode == ModeAuto && l.hrConnected {
		l.targetSpeed = clampPercent(l.mapper.Map(l.effectiveHRLocked()))
		if l.running {
			l.maybeApplyLocked(ctx, true)
		}
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// SetManualSpeed switches to ModeManual and applies the requested speed (0..100) if running.
func (l *Loop) SetManualSpeed(ctx context.Context, percent int) Snapshot {
	l.mu.Lock()
	l.mode = ModeManual
	l.manualSpeed = clampPercent(percent)
	l.targetSpeed = l.manualSpeed
	if l.running {
		l.maybeApplyLocked(ctx, true)
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

// CheckTimeout verifies whether the HR stream has gone stale and turns the fan off in AUTO mode.
func (l *Loop) CheckTimeout(ctx context.Context) Snapshot {
	l.mu.Lock()
	now := l.nowFn()
	if l.hrConnected && !l.lastHRAt.IsZero() && now.Sub(l.lastHRAt) > l.hrTimeout {
		l.hrConnected = false
		l.bpm = 0
		l.effectiveBPM = 0
		l.window.Reset()
		if l.mode == ModeAuto {
			l.targetSpeed = 0
			if l.running {
				l.maybeApplyLocked(ctx, true)
			}
		}
	}
	snap := l.snapshotLocked()
	l.mu.Unlock()
	return snap
}

func (l *Loop) maybeApplyLocked(ctx context.Context, force bool) {
	if l.fan == nil {
		return
	}
	now := l.nowFn()
	delta := l.targetSpeed - l.appliedSpeed
	if delta < 0 {
		delta = -delta
	}

	if !force && l.hasSent {
		if l.targetSpeed == l.appliedSpeed {
			return
		}
		if delta < l.minDelta && l.targetSpeed != 0 {
			return
		}
		if now.Sub(l.lastSentAt) < l.minInterval {
			return
		}
	}

	var err error
	if l.targetSpeed == 0 {
		err = l.fan.Off(ctx)
	} else {
		err = l.fan.SetSpeed(ctx, l.targetSpeed)
	}
	if err != nil {
		l.lastErr = err.Error()
		return
	}
	l.lastErr = ""
	l.appliedSpeed = l.targetSpeed
	l.lastSentAt = now
	l.hasSent = true
}

func (l *Loop) effectiveHRLocked() uint8 {
	if l.effectiveBPM > 0 {
		return l.effectiveBPM
	}
	return l.bpm
}

func (l *Loop) snapshotLocked() Snapshot {
	return Snapshot{
		Running:       l.running,
		BPM:           l.bpm,
		EffectiveBPM:  l.effectiveBPM,
		WindowCount:   l.window.Count(),
		WindowSeconds: l.window.Capacity(),
		Percentile:    l.percentile,
		LastHRAt:      l.lastHRAt,
		TargetSpeed:   l.targetSpeed,
		AppliedSpeed:  l.appliedSpeed,
		Mode:          l.mode,
		HRConnected:   l.hrConnected,
		LastErr:       l.lastErr,
	}
}
