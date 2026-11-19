package common

import (
	"testing"
	"time"
)

func TestFinancialWaitIncludesNestedRefundCacheWrites(t *testing.T) {
	started, release, nested := make(chan struct{}), make(chan struct{}), make(chan struct{})
	GoFinancial(func() {
		close(started)
		<-release
		GoFinancial(func() { <-nested })
	})
	<-started
	waited := make(chan struct{})
	go func() { FinancialWork.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("refund was not awaited")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-waited:
		t.Fatal("nested cache write was not awaited")
	case <-time.After(20 * time.Millisecond):
	}
	close(nested)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("refund wait deadlocked")
	}
}
