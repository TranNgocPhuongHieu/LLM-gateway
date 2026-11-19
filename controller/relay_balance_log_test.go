package controller

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/types"
)

func TestBalanceRejectionLogLimiterAllowsOncePerWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newBalanceRejectionLogLimiter(func() time.Time { return now })

	if !l.allow(7, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("first rejection must be recorded")
	}
	now = now.Add(59 * time.Second)
	if l.allow(7, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("second rejection inside the window must be skipped")
	}
	if !l.allow(8, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("another user is limited independently")
	}
	if !l.allow(7, types.ErrorCodePreConsumeTokenQuotaFailed) {
		t.Fatal("another error code is limited independently")
	}
	now = now.Add(time.Second)
	if !l.allow(7, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("rejection after the window must be recorded again")
	}
	if l.allow(0, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("anonymous requests are never recorded")
	}
}

func TestBalanceRejectionLogLimiterBoundsMemory(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newBalanceRejectionLogLimiter(func() time.Time { return now })
	for i := 1; i <= balanceRejectionLogMaxEntries; i++ {
		l.allow(i, types.ErrorCodeInsufficientUserQuota)
	}
	now = now.Add(balanceRejectionLogWindow)
	if !l.allow(balanceRejectionLogMaxEntries+1, types.ErrorCodeInsufficientUserQuota) {
		t.Fatal("new user must be recorded")
	}
	if got := len(l.last); got != 1 {
		t.Fatalf("stale entries must be pruned, got %d entries", got)
	}
}

func TestShouldRecordBalanceRejectionOnlyForBalanceCodes(t *testing.T) {
	saved := balanceRejectionLimiter
	t.Cleanup(func() { balanceRejectionLimiter = saved })
	now := time.Unix(1_700_000_000, 0)
	balanceRejectionLimiter = newBalanceRejectionLogLimiter(func() time.Time { return now })

	balance := types.NewErrorWithStatusCode(errors.New("用户额度不足, 剩余额度: 0"),
		types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
		types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	if types.IsRecordErrorLog(balance) {
		t.Fatal("precondition: balance errors opt out of the default error log")
	}
	if !shouldRecordBalanceRejection(42, balance) {
		t.Fatal("first balance rejection must be recorded")
	}
	if shouldRecordBalanceRejection(42, balance) {
		t.Fatal("repeated balance rejection must be rate limited")
	}
	other := types.NewError(errors.New("bad"), types.ErrorCodeInvalidRequest, types.ErrOptionWithNoRecordErrorLog())
	if shouldRecordBalanceRejection(43, other) {
		t.Fatal("non-balance errors that opted out must stay unrecorded")
	}
	if shouldRecordBalanceRejection(44, nil) {
		t.Fatal("nil error must not be recorded")
	}
}
