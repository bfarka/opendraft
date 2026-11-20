package control

import (
	"reflect"
	"testing"
	"time"
)

func TestHRWindowPercentileAndEviction(t *testing.T) {
	base := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	w := NewHRWindow(10)

	if got := w.Percentile(80); got != 0 {
		t.Fatalf("empty window Percentile(80) = %d, want 0", got)
	}

	// Add 10 samples: 101..110 (80th percentile = rank ceil(0.8*10) = 8 -> 108).
	for i := 0; i < 10; i++ {
		w.Add(uint8(101+i), base.Add(time.Duration(i)*time.Second))
	}
	if w.Count() != 10 {
		t.Fatalf("Count() = %d, want 10", w.Count())
	}
	if got := w.Percentile(80); got != 108 {
		t.Fatalf("Percentile(80) = %d, want 108", got)
	}

	// Advance 5 seconds with 120 bpm: 101..105 are evicted, window has [106..110, 120x5].
	for i := 10; i < 15; i++ {
		w.Add(120, base.Add(time.Duration(i)*time.Second))
	}
	if w.Count() != 10 {
		t.Fatalf("Count() after overwrite = %d, want 10", w.Count())
	}
	if got := w.Percentile(80); got != 120 {
		t.Fatalf("Percentile(80) after shift = %d, want 120", got)
	}

	// Gap of 4 seconds (t=19): 4 slots cleared (0), 1 slot set to 130.
	// Remaining non-zero: 1x110 (at t=9? no, t=10..14 were 120, t=15..18 cleared, t=19=130).
	// Wait: window covers t=10..19, so t=10..14 are 120 (5 samples) + t=19 is 130 (1 sample) = 6 samples.
	w.Add(130, base.Add(19*time.Second))
	if w.Count() != 6 {
		t.Fatalf("Count() after 4s gap = %d, want 6", w.Count())
	}
	// Rank ceil(0.8 * 6) = 5 -> 5th sample is 120.
	if got := w.Percentile(80); got != 120 {
		t.Fatalf("Percentile(80) with 6 samples = %d, want 120", got)
	}

	// Gap >= window size resets previous samples.
	w.Add(140, base.Add(40*time.Second))
	if w.Count() != 1 || w.Percentile(80) != 140 {
		t.Fatalf("after full expiry: count=%d p80=%d, want count=1 p80=140", w.Count(), w.Percentile(80))
	}
}

func TestHRWindowHistoryAndResize(t *testing.T) {
	base := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	w := NewHRWindow(5)
	for i := 0; i < 5; i++ {
		w.Add(uint8(100+i), base.Add(time.Duration(i)*time.Second))
	}
	got := w.History(nil)
	want := []uint8{100, 101, 102, 103, 104}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("History() = %v, want %v", got, want)
	}

	// Shrink to 3 keeps most recent 3.
	w.Resize(3)
	got = w.History(nil)
	want = []uint8{102, 103, 104}
	if !reflect.DeepEqual(got, want) || w.Count() != 3 {
		t.Fatalf("after Resize(3): hist=%v count=%d, want %v count=3", got, w.Count(), want)
	}

	// Zero BPM is ignored.
	w.Add(0, base.Add(5*time.Second))
	if w.Count() != 3 {
		t.Fatalf("zero BPM should be ignored, count=%d", w.Count())
	}

	// Reset clears everything.
	w.Reset()
	if w.Count() != 0 || len(w.History(nil)) != 0 || w.Percentile(80) != 0 {
		t.Fatalf("expected empty window after Reset")
	}
}

func TestHRWindowZeroAllocs(t *testing.T) {
	base := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	w := NewHRWindow(120)
	for i := 0; i < 120; i++ {
		w.Add(uint8(110+(i%40)), base.Add(time.Duration(i)*time.Second))
	}

	allocs := testing.AllocsPerRun(100, func() {
		base = base.Add(time.Second)
		w.Add(145, base)
		_ = w.Percentile(80)
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocs per Add+Percentile, got %v", allocs)
	}
}
