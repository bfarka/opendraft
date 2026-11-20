package control

import "time"

// HRWindow maintains a memory-efficient rolling 1-second heart rate window
// using a fixed 1-byte-per-second ring buffer and a 256-bucket BPM histogram.
// Steady-state updates and percentile queries perform zero heap allocations.
type HRWindow struct {
	slots   []uint8
	hist    [256]uint16
	lastSec int64
	head    int
	count   int
	hasData bool
}

// NewHRWindow creates an HRWindow with capacity for windowSeconds 1-second buckets.
func NewHRWindow(windowSeconds int) *HRWindow {
	if windowSeconds <= 0 {
		windowSeconds = 120
	}
	return &HRWindow{
		slots: make([]uint8, windowSeconds),
	}
}

// Capacity returns the configured window size in seconds.
func (w *HRWindow) Capacity() int {
	return len(w.slots)
}

// Count returns the number of non-zero HR samples currently inside the window.
func (w *HRWindow) Count() int {
	return w.count
}

// Reset clears all samples and histogram counts from the window.
func (w *HRWindow) Reset() {
	clear(w.slots)
	clear(w.hist[:])
	w.lastSec = 0
	w.head = 0
	w.count = 0
	w.hasData = false
}

// Resize updates the window capacity while preserving the most recent samples.
func (w *HRWindow) Resize(windowSeconds int) {
	if windowSeconds <= 0 {
		windowSeconds = 120
	}
	if windowSeconds == len(w.slots) {
		return
	}
	if !w.hasData {
		w.slots = make([]uint8, windowSeconds)
		return
	}

	old := w.History(nil)
	w.slots = make([]uint8, windowSeconds)
	clear(w.hist[:])
	w.count = 0

	start := 0
	if len(old) > windowSeconds {
		start = len(old) - windowSeconds
	}
	n := len(old) - start
	offset := windowSeconds - n
	for i := 0; i < n; i++ {
		bpm := old[start+i]
		w.slots[offset+i] = bpm
		if bpm > 0 {
			w.hist[bpm]++
			w.count++
		}
	}
	w.head = windowSeconds - 1
}

// Add records a heart rate measurement at ts into the 1-second ring buffer.
// Zero BPM values are ignored.
func (w *HRWindow) Add(bpm uint8, ts time.Time) {
	if bpm == 0 {
		return
	}
	sec := ts.Unix()
	n := len(w.slots)

	if !w.hasData {
		w.hasData = true
		w.lastSec = sec
		w.head = 0
		w.setSlot(0, bpm)
		return
	}

	delta := sec - w.lastSec
	if delta < 0 {
		back := int(-delta)
		if back >= n {
			return
		}
		idx := (w.head - back + n) % n
		w.setSlot(idx, bpm)
		return
	}

	if delta == 0 {
		w.setSlot(w.head, bpm)
		return
	}

	if delta >= int64(n) {
		w.Reset()
		w.hasData = true
		w.lastSec = sec
		w.head = 0
		w.setSlot(0, bpm)
		return
	}

	steps := int(delta)
	for i := 0; i < steps-1; i++ {
		w.head = (w.head + 1) % n
		w.setSlot(w.head, 0)
	}
	w.head = (w.head + 1) % n
	w.setSlot(w.head, bpm)
	w.lastSec = sec
}

// Percentile returns the p-th percentile (1..100) of the samples currently in the window
// using the nearest-rank method in O(256) time with zero allocations.
// Returns 0 if the window is empty.
func (w *HRWindow) Percentile(p int) uint8 {
	if w.count == 0 {
		return 0
	}
	if p <= 0 {
		p = 1
	} else if p > 100 {
		p = 100
	}

	rank := (p*w.count + 99) / 100
	if rank < 1 {
		rank = 1
	}

	cum := 0
	for bpm := 1; bpm <= 255; bpm++ {
		cum += int(w.hist[bpm])
		if cum >= rank {
			return uint8(bpm)
		}
	}
	return 0
}

// History appends the window's 1-second buckets in chronological order (oldest to newest)
// to dst and returns the resulting slice. Empty buckets are 0.
func (w *HRWindow) History(dst []uint8) []uint8 {
	dst = dst[:0]
	if !w.hasData {
		return dst
	}
	n := len(w.slots)
	start := (w.head + 1) % n
	for i := 0; i < n; i++ {
		dst = append(dst, w.slots[(start+i)%n])
	}
	return dst
}

func (w *HRWindow) setSlot(idx int, bpm uint8) {
	old := w.slots[idx]
	if old == bpm {
		return
	}
	if old > 0 {
		w.hist[old]--
		w.count--
	}
	w.slots[idx] = bpm
	if bpm > 0 {
		w.hist[bpm]++
		w.count++
	}
}
