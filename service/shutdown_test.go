package service

import (
	"sync"
	"testing"
	"time"
)

func TestSystemTaskStopWaitsForClaimedWork(t *testing.T) {
	oldStop := systemTaskStop
	systemTaskStop = make(chan struct{})
	systemTaskStopOnce = sync.Once{}
	t.Cleanup(func() { systemTaskStop = oldStop; systemTaskStopOnce = sync.Once{} })
	systemTaskWork.Add(1)
	finished := make(chan struct{})
	go func() { StopSystemTaskRunner(); close(finished) }()
	<-systemTaskStop
	select {
	case <-finished:
		t.Fatal("exited with a claimed task running")
	case <-time.After(20 * time.Millisecond):
	}
	systemTaskWork.Done()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("stop deadlocked")
	}
}
