package ble

import (
	"context"
	"sync"
	"testing"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/bfarka/opendraft/internal/domain"
)

func TestHRStreamConcurrentSendAndClose(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := newHRStream(4)
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for n := 0; n < 50; n++ {
					s.send(domain.HRReading{BPM: uint8(60 + id), Timestamp: time.Now()})
				}
			}(g)
		}

		s.close()
		s.close() // idempotent
		wg.Wait()
	}
}

func TestEmitConnectedRegistersAddressesAndDeduplicates(t *testing.T) {
	id1 := "dba1e49d-4fb3-4187-e610-77b59cc93f5d"
	id2 := "a49579d0-110b-571b-48e5-e5aff9942f15"

	src := &Source{
		addresses: make(map[string]bluetooth.Address),
		connectedFn: func(_ context.Context) []domain.HRDevice {
			return []domain.HRDevice{
				{ID: id1, Name: "TICKR FIT 4C31"},
				{ID: id1, Name: "TICKR FIT 4C31"}, // duplicate
				{ID: id2, Name: "Polar H10"},
			}
		},
	}

	out := make(chan domain.HRDevice, 4)
	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	ok := src.emitConnected(context.Background(), out, seen, &seenMu)
	if !ok {
		t.Fatal("emitConnected returned false on active context")
	}
	close(out)

	var got []domain.HRDevice
	for d := range out {
		got = append(got, d)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deduplicated devices, got %d: %+v", len(got), got)
	}
	if got[0].ID != id1 || got[1].ID != id2 {
		t.Fatalf("unexpected devices: %+v", got)
	}
	if _, ok := src.addresses[id1]; !ok {
		t.Fatalf("expected address %s to be registered", id1)
	}
	if _, ok := src.addresses[id2]; !ok {
		t.Fatalf("expected address %s to be registered", id2)
	}
}
