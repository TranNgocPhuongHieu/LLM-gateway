package common

import (
	"github.com/bytedance/gopkg/util/gopool"
	"sync"
)

// FinancialWork tracks asynchronous quota writes (including nested cache writes).
// Wait only after HTTP handlers and other producers have stopped. Register work
// before launching its goroutine, never from inside the new goroutine.
var FinancialWork sync.WaitGroup

func GoFinancial(fn func()) {
	FinancialWork.Add(1)
	gopool.Go(func() { defer FinancialWork.Done(); fn() })
}
