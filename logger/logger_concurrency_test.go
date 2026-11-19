package logger

import (
	"sync"
	"testing"
)

func TestConcurrentStreamLoggingIsRaceFree(t *testing.T) {
	const requests = 300
	var wg sync.WaitGroup
	wg.Add(requests)
	for i := 0; i < requests; i++ {
		go func() {
			defer wg.Done()
			LogInfo(nil, "stream ended: mock")
		}()
	}
	wg.Wait()
}
