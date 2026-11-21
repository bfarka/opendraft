package ble

import (
	"sync"
	"testing"
	"time"

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
