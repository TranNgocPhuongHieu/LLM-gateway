package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

func TestClientRejectionLogLimiterAllowsOncePerWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newClientRejectionLogLimiter(func() time.Time { return now })
	code := types.ErrorCodeInsufficientUserQuota

	if ok, _ := l.allow(7, code, ""); !ok {
		t.Fatal("first rejection must be logged")
	}
	for range 3 {
		now = now.Add(time.Minute)
		if ok, _ := l.allow(7, code, ""); ok {
			t.Fatal("repeated rejection inside the window must be skipped")
		}
	}
	if ok, _ := l.allow(8, code, ""); !ok {
		t.Fatal("another user is limited independently")
	}
	if ok, _ := l.allow(7, types.ErrorCodePreConsumeTokenQuotaFailed, ""); !ok {
		t.Fatal("another error code is limited independently")
	}
	if ok, _ := l.allow(7, types.ErrorCodeModelNotFound, "a"); !ok {
		t.Fatal("first unavailable model must be logged")
	}
	if ok, _ := l.allow(7, types.ErrorCodeModelNotFound, "b"); !ok {
		t.Fatal("another model is limited independently")
	}
	now = now.Add(clientRejectionLogWindow)
	ok, suppressed := l.allow(7, code, "")
	if !ok {
		t.Fatal("rejection after the window must be logged again")
	}
	if suppressed != 3 {
		t.Fatalf("suppressed = %d, want 3", suppressed)
	}
	if _, suppressed := l.allow(7, types.ErrorCodeModelNotFound, "a"); suppressed != 0 {
		t.Fatalf("suppressed = %d, want 0", suppressed)
	}
	if ok, _ := l.allow(0, code, ""); ok {
		t.Fatal("anonymous requests are never logged")
	}
}

func TestClientRejectionLogLimiterBoundsMemory(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newClientRejectionLogLimiter(func() time.Time { return now })
	for i := 1; i <= clientRejectionLogMaxEntries; i++ {
		l.allow(i, types.ErrorCodeInsufficientUserQuota, "")
	}
	now = now.Add(clientRejectionLogWindow)
	if ok, _ := l.allow(clientRejectionLogMaxEntries+1, types.ErrorCodeInsufficientUserQuota, ""); !ok {
		t.Fatal("new user must be logged")
	}
	if got := len(l.entries); got != 1 {
		t.Fatalf("stale entries must be pruned, got %d entries", got)
	}
}

func TestIsBalanceRejection(t *testing.T) {
	balance := types.NewErrorWithStatusCode(errors.New("Insufficient balance"),
		types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
		types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	if types.IsRecordErrorLog(balance) {
		t.Fatal("precondition: balance errors opt out of the default error log")
	}
	if !IsBalanceRejection(balance) {
		t.Fatal("wallet balance rejection must be recognised")
	}
	key := types.NewErrorWithStatusCode(errors.New("key quota"), types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden)
	if !IsBalanceRejection(key) {
		t.Fatal("key balance rejection must be recognised")
	}
	if IsBalanceRejection(types.NewError(errors.New("bad"), types.ErrorCodeInvalidRequest)) {
		t.Fatal("other errors are not balance rejections")
	}
	if IsBalanceRejection(nil) {
		t.Fatal("nil is not a balance rejection")
	}
}
