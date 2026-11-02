package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestValidateReadOnlyRestrictions(t *testing.T) {
	if err := ValidateReadOnlyRestrictions(BinanceAccountRestrictions{EnableReading: true, IPRestrict: true}); err != nil {
		t.Fatalf("read-only key rejected: %v", err)
	}
	for name, r := range map[string]BinanceAccountRestrictions{
		"withdraw": {EnableReading: true, IPRestrict: true, EnableWithdrawals: true},
		"transfer": {EnableReading: true, IPRestrict: true, EnableInternalTransfer: true},
		"trading":  {EnableReading: true, IPRestrict: true, EnableSpotAndMargin: true},
		"no-read":  {},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateReadOnlyRestrictions(r); err == nil {
				t.Fatal("dangerous or unusable key accepted")
			}
		})
	}
}

func TestBinanceReadOnlyAllowlist(t *testing.T) {
	for _, path := range []string{
		"/sapi/v1/account/apiRestrictions",
		"/sapi/v1/capital/deposit/hisrec",
		"/sapi/v1/capital/deposit/address",
	} {
		if !BinanceReadOnlyRequest("GET", path) {
			t.Fatalf("allowed path rejected: %s", path)
		}
	}
	for _, path := range []string{
		"/sapi/v1/capital/withdraw/apply",
		"https://api.binance.com/sapi/v1/capital/deposit/hisrec",
	} {
		if BinanceReadOnlyRequest("GET", path) {
			t.Fatalf("non-allowlisted path accepted: %s", path)
		}
	}
	if !BinanceReadOnlyRequest("GET", "/sapi/v1/account/apiRestrictions?timestamp=1") {
		t.Fatal("signed read-only request with query rejected")
	}
	if BinanceReadOnlyRequest("POST", "/sapi/v1/capital/deposit/hisrec") {
		t.Fatal("non-GET Binance request accepted")
	}
}

func TestBinancePhaseOneFlagsFailClosed(t *testing.T) {
	flags := DefaultBinanceFeatureFlags
	if flags.CheckoutEnabled || flags.RealPollingEnabled || flags.AutoSettlementEnabled || !flags.MockMode || !flags.IngestOnly {
		t.Fatalf("unsafe phase-1 defaults: %+v", flags)
	}
}

func TestBinanceHTTPClientSignsAllowlistedGETWithoutNetwork(t *testing.T) {
	var got *http.Request
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Date": []string{"Mon, 02 Jan 2006 15:04:05 GMT"}}, Body: io.NopCloser(strings.NewReader(`{"enableReading":true,"ipRestrict":true}`))}, nil
	})}
	c, err := NewBinanceHTTPClient(BinanceHTTPClientConfig{APIKey: "public-fixture", APISecret: "private-fixture", BaseURL: BinanceProductionBaseURL, HTTPClient: client, Now: func() time.Time { return time.Unix(1700000000, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.AccountRestrictions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.EnableReading || got == nil {
		t.Fatalf("unexpected response/request: %+v", got)
	}
	if got.Method != http.MethodGet || got.Header.Get("X-MBX-APIKEY") != "public-fixture" || got.URL.Host != "api.binance.com" {
		t.Fatalf("unsafe request: %s %s", got.Method, got.URL.String())
	}
	q := got.URL.Query()
	if q.Get("timestamp") == "" || q.Get("recvWindow") == "" || q.Get("signature") == "" {
		t.Fatalf("missing signature parameters: %v", q)
	}
	if strings.Contains(got.URL.String(), "private-fixture") {
		t.Fatal("secret leaked in URL")
	}
}

func TestBinanceHTTPClientRejectsNonBinanceBaseAndRedirect(t *testing.T) {
	if _, err := NewBinanceHTTPClient(BinanceHTTPClientConfig{APIKey: "k", APISecret: "s", BaseURL: "https://evil.example"}); err == nil {
		t.Fatal("untrusted base URL accepted")
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.example/collect"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})}
	c, err := NewBinanceHTTPClient(BinanceHTTPClientConfig{APIKey: "k", APISecret: "s", BaseURL: BinanceProductionBaseURL, HTTPClient: client, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CapitalConfig(context.Background()); err == nil {
		t.Fatal("redirect response accepted")
	}
}

func TestBinanceHTTPClientRetries429AndHonorsContext(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"enableReading":true,"ipRestrict":true}`))}, nil
	})}
	c, err := NewBinanceHTTPClient(BinanceHTTPClientConfig{APIKey: "k", APISecret: "s", BaseURL: BinanceProductionBaseURL, HTTPClient: client, MaxAttempts: 3, RetryBase: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccountRestrictions(context.Background()); err != nil || calls != 3 {
		t.Fatalf("bounded retry failed: calls=%d err=%v", calls, err)
	}
}

func TestBinanceDepositParsingRejectsPrecisionAndMissingIdentity(t *testing.T) {
	if _, err := parseUSDTUnits("1.0000001"); err == nil {
		t.Fatal("amount over six decimals accepted")
	}
	if got, err := parseUSDTUnits("1.230000"); err != nil || got != 1230000 {
		t.Fatalf("unexpected decimal parse: %d %v", got, err)
	}
	if err := ValidateBinanceNetworkAddress(BinanceNetworkConfig{Coin: BinanceCoinUSDT, Network: "BSC", DepositEnable: true}, BinanceDepositAddress{Coin: BinanceCoinUSDT, Network: "BSC", Address: "0xgood"}, "BSC", "0xbad"); err == nil {
		t.Fatal("mismatched configured address accepted")
	}
}

func TestBinanceDepositHistoryParsesIdentityAndTravelRule(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"id":"dep-1","amount":"10.250000","coin":"USDT","network":"BSC","status":1,"address":"0xgood","txId":"tx-1","insertTime":1700000000000,"confirmTimes":"20","travelRuleStatus":"INCOMPLETE"}]`)), Header: make(http.Header)}, nil
	})}
	c, err := NewBinanceHTTPClient(BinanceHTTPClientConfig{APIKey: "k", APISecret: "s", BaseURL: BinanceProductionBaseURL, HTTPClient: client, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.DepositHistory(context.Background(), "BSC", time.UnixMilli(1699999999000), time.UnixMilli(1700000001000))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ProviderDepositID != "dep-1" || rows[0].AmountUnits != 10250000 || rows[0].TravelRuleStatus != "INCOMPLETE" {
		t.Fatalf("unexpected parsed deposit: %+v", rows)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBuildBinanceQuoteAndUniqueAmount(t *testing.T) {
	q, err := BuildBinanceQuote(QuoteInput{BaseUSDT: "10", FeePercent: "2.5", USDTToVND: "26000", QuotaPerUnit: "500000", BonusPercent: "10", MinUSDT: "10", MaxUSDT: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if q.AmountUnits != 10250000 || q.AmountVND != 260000 || q.Credits != 286 {
		t.Fatalf("unexpected quote: %+v", q)
	}
	allocated, err := AllocateUniqueAmount(q.AmountUnits, map[int64]struct{}{q.AmountUnits: {}})
	if err != nil || allocated != q.AmountUnits+1 {
		t.Fatalf("unexpected allocation: %d %v", allocated, err)
	}
}

func TestBuildBinanceQuoteAddsFeeWithoutGrantingCreditOnFee(t *testing.T) {
	q, err := BuildBinanceQuote(QuoteInput{BaseUSDT: "20", FeePercent: "2", USDTToVND: "26000", QuotaPerUnit: "500000", BonusPercent: "10", MinUSDT: "10", MaxUSDT: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if q.PayableUSDT != "20.400000" || q.AmountVND != 520000 || q.Credits != 572 {
		t.Fatalf("fee/bonus calculation incorrect: %+v", q)
	}
}

func TestMockBinanceClientFiltersByNetworkAndTime(t *testing.T) {
	now := time.Unix(1000, 0)
	mock := &MockBinanceWalletClient{Restrictions: BinanceAccountRestrictions{EnableReading: true, IPRestrict: true}, Deposits: []BinanceDepositRecord{
		{ProviderDepositID: "a", Network: "BEP20", InsertTime: now.Add(time.Minute)},
		{ProviderDepositID: "b", Network: "TRC20", InsertTime: now.Add(time.Minute)},
	}}
	rows, err := mock.DepositHistory(context.Background(), "BEP20", now, now.Add(2*time.Minute))
	if err != nil || len(rows) != 1 || rows[0].ProviderDepositID != "a" || mock.Calls != 1 {
		t.Fatalf("unexpected mock result: %+v calls=%d err=%v", rows, mock.Calls, err)
	}
}
