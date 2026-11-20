package service

import (
	"strconv"
	"sync"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

// Client rejections are failures a retry cannot fix: an empty wallet or key
// balance, or a model no channel offers. Clients often retry them in a loop,
// so they are logged as warnings and written to the error log at most once
// per user, error code and model per window. Customers still see one row on
// their dashboard per window.
const clientRejectionLogWindow = 10 * time.Minute

// clientRejectionLogMaxEntries bounds the limiter map; when reached, entries
// older than the window are pruned (and the map is reset if none are stale).
const clientRejectionLogMaxEntries = 10000

var balanceRejectionCodes = map[types.ErrorCode]struct{}{
	types.ErrorCodeInsufficientUserQuota:      {},
	types.ErrorCodePreConsumeTokenQuotaFailed: {},
}

// IsBalanceRejection reports whether err rejects a request because the
// wallet or the key has too little balance.
func IsBalanceRejection(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	_, ok := balanceRejectionCodes[err.GetErrorCode()]
	return ok
}

type clientRejectionEntry struct {
	last       time.Time
	suppressed int
}

type clientRejectionLogLimiter struct {
	mu      sync.Mutex
	entries map[string]*clientRejectionEntry
	now     func() time.Time
}

var clientRejectionLimiter = newClientRejectionLogLimiter(time.Now)

func newClientRejectionLogLimiter(now func() time.Time) *clientRejectionLogLimiter {
	return &clientRejectionLogLimiter{entries: make(map[string]*clientRejectionEntry), now: now}
}

// allow reports whether this rejection should be logged now and how many
// identical rejections were skipped since the last logged one.
func (l *clientRejectionLogLimiter) allow(userID int, code types.ErrorCode, modelName string) (bool, int) {
	if userID <= 0 {
		return false, 0
	}
	key := strconv.Itoa(userID) + "\x00" + string(code) + "\x00" + modelName
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	if entry, ok := l.entries[key]; ok && now.Sub(entry.last) < clientRejectionLogWindow {
		entry.suppressed++
		return false, 0
	}
	suppressed := 0
	if entry, ok := l.entries[key]; ok {
		suppressed = entry.suppressed
	}
	if _, ok := l.entries[key]; !ok && len(l.entries) >= clientRejectionLogMaxEntries {
		for k, entry := range l.entries {
			if now.Sub(entry.last) >= clientRejectionLogWindow {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= clientRejectionLogMaxEntries {
			l.entries = make(map[string]*clientRejectionEntry)
		}
	}
	l.entries[key] = &clientRejectionEntry{last: now}
	return true, suppressed
}

// AllowClientRejectionLog reports whether a client rejection for this user,
// code and model should be logged now, and how many identical rejections
// were skipped since the previous logged one. Anonymous requests are never
// logged.
func AllowClientRejectionLog(userID int, code types.ErrorCode, modelName string) (bool, int) {
	return clientRejectionLimiter.allow(userID, code, modelName)
}
