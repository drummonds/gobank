package refs

import (
	"sync"
	"testing"
)

// A sequence continues after the last number on record, and a number
// once allocated is never given again, however many allocate at once.
func TestSequenceContinuesAndNeverRepeats(t *testing.T) {
	s := Resume(41)
	if n := s.Next(); n != 42 {
		t.Fatalf("Next after 41 = %d, want 42", n)
	}
	const workers, each = 8, 100
	var wg sync.WaitGroup
	got := make(chan int, workers*each)
	for range workers {
		wg.Go(func() {
			for range each {
				got <- s.Next()
			}
		})
	}
	wg.Wait()
	close(got)
	seen := map[int]bool{}
	for n := range got {
		if seen[n] || n < 43 || n > 42+workers*each {
			t.Errorf("number %d repeated or out of range", n)
		}
		seen[n] = true
	}
	if len(seen) != workers*each {
		t.Errorf("%d distinct numbers, want %d", len(seen), workers*each)
	}
	if Resume(0).Next() != 1 {
		t.Error("an empty record numbers from one")
	}
}
