package controller

import (
	"strconv"
	"sync"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

// Balance rejections are created with ErrOptionWithNoRecordErrorLog because a
// client that keeps retrying with an empty wallet would otherwise write one
// error row per attempt. Customers still need to see them on the dashboard, so
// at most one row per user and error code is recorded per window.
const balanceRejectionLogWindow = time.Minute

// balanceRejectionLogMaxEntries bounds the limiter map; when reached, entries
// older than the window are pruned (and the map is reset if none are stale).
const balanceRejectionLogMaxEntries = 10000

var balanceRejectionLogCodes = map[types.ErrorCode]struct{}{
	types.ErrorCodeInsufficientUserQuota:      {},
	types.ErrorCodePreConsumeTokenQuotaFailed: {},
}

type balanceRejectionLogLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
	now  func() time.Time
}

var balanceRejectionLimiter = newBalanceRejectionLogLimiter(time.Now)

func newBalanceRejectionLogLimiter(now func() time.Time) *balanceRejectionLogLimiter {
	return &balanceRejectionLogLimiter{last: make(map[string]time.Time), now: now}
}

// allow reports whether a balance rejection for this user and code should be
// written now, and records the write time when it returns true.
func (l *balanceRejectionLogLimiter) allow(userID int, code types.ErrorCode) bool {
	if userID <= 0 {
		return false
	}
	key := strconv.Itoa(userID) + "\x00" + string(code)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.last[key]; ok && now.Sub(last) < balanceRejectionLogWindow {
		return false
	}
	if len(l.last) >= balanceRejectionLogMaxEntries {
		for k, t := range l.last {
			if now.Sub(t) >= balanceRejectionLogWindow {
				delete(l.last, k)
			}
		}
		if len(l.last) >= balanceRejectionLogMaxEntries {
			l.last = make(map[string]time.Time)
		}
	}
	l.last[key] = now
	return true
}

// shouldRecordBalanceRejection is true for wallet/key balance rejections that
// pass the per-user rate limit.
func shouldRecordBalanceRejection(userID int, err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	code := err.GetErrorCode()
	if _, ok := balanceRejectionLogCodes[code]; !ok {
		return false
	}
	return balanceRejectionLimiter.allow(userID, code)
}
