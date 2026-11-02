package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
)

func TestCustomerRequestCostIncludesXPikiBefore9router(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/status" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"quota_used": 0, "quota_limit": 1, "conversion": json.RawMessage(gptConversionFixture)}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{
			{"request_id": "charged-attempt", "created_at": "2026-10-10T08:00:00+07:00", "model_slug": "gpt-6.1-sol", "status": "failed", "input_tokens": 1000, "output_tokens": 0, "credit_cost": 120},
		}}})
	}))
	defer server.Close()
	credentials := []XPikiQuotaCredential{{ChannelId: 8, APIKey: "test-key", BaseURL: "https://api.xpiki.com"}}
	makeReport := func() XPikiLogsReport {
		return XPikiLogsReport{Items: []XPikiLogItem{{LogID: 2, ChannelId: 10, Destination: "9router:20128", Status: "success", InputTokens: 2000, OutputTokens: 50,
			Attempts: []XPikiLogItem{
				{LogID: 1, ChannelId: 8, Destination: "api.xpiki.com", Status: "failed", ModelSlug: "gpt-6.1-sol", CreatedAt: "2026-10-10T08:00:00+07:00", UpstreamRequestID: "charged-attempt", QuotaSource: "unmatched"},
				{LogID: 2, ChannelId: 10, Destination: "9router:20128", Status: "success"},
			},
		}}}
	}
	report := makeReport()
	enrichLocalRequestCosts(context.Background(), &report, credentials, server.URL+"/logs", server.URL+"/status")
	item := report.Items[0]
	if item.TokenCost == nil || item.TokenCost.Status != "reconciled" || item.TokenCost.TotalVND != "0.624" || item.QuotaSource != "xpiki" {
		t.Fatalf("lost charged XPiki fallback cost: %+v", item)
	}
	if item.ChannelId != 10 || item.InputTokens != 2000 || item.OutputTokens != 50 || len(item.Attempts) != 2 {
		t.Fatal("cost enrichment changed customer request identity/usage")
	}
	summarizeXPikiTokenCosts(&report)
	if report.TokenCostSummary.TotalVND != "0.624" || report.TokenCostSummary.KnownRequests != 1 {
		t.Fatalf("wrong request total: %+v", report.TokenCostSummary)
	}
	before := calls.Load()
	report = makeReport()
	enrichLocalRequestCosts(context.Background(), &report, credentials, server.URL+"/logs", server.URL+"/status")
	if calls.Load() != before || report.Items[0].TokenCost.TotalVND != "0.624" {
		t.Fatal("10s polling queried upstream again or lost cached cost")
	}
	// A changed key is a different configuration and must invalidate receipts.
	credentials[0].APIKey = "changed-test-key"
	report = makeReport()
	enrichLocalRequestCosts(context.Background(), &report, credentials, server.URL+"/logs", server.URL+"/status")
	if calls.Load() <= before {
		t.Fatal("changed key reused a cached receipt")
	}
}

func TestCustomerRequestCostNeverTreatsUnknownAttemptsAsFree(t *testing.T) {
	known := &model.XPikiTokenCost{Status: "reconciled", TotalVND: "1.005"}
	for _, tc := range []struct {
		name                  string
		attempts              []XPikiLogItem
		status, total, source string
	}{
		{"9router-only", []XPikiLogItem{{QuotaSource: "not_xpiki"}}, "", "", "not_xpiki"},
		{"unknown-XPiki-before-9router", []XPikiLogItem{{QuotaSource: "unmatched"}, {QuotaSource: "not_xpiki"}}, "unverified", "", "unmatched"},
		{"one-known-one-unknown", []XPikiLogItem{{QuotaSource: "xpiki", TokenCost: known}, {QuotaSource: "unmatched"}}, "unverified", "", "unmatched"},
		{"sum-original-decimals", []XPikiLogItem{{QuotaSource: "xpiki", TokenCost: known}, {QuotaSource: "xpiki", TokenCost: known}}, "reconciled", "2.01", "xpiki"},
		{"zero-receipts", []XPikiLogItem{{QuotaSource: "xpiki", TokenCost: &model.XPikiTokenCost{Status: "confirmed_zero", TotalVND: "0"}}, {QuotaSource: "not_xpiki"}}, "confirmed_zero", "0", "xpiki"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, source := aggregateRequestAttemptCosts(tc.attempts)
			if source != tc.source {
				t.Fatalf("source %s != %s", source, tc.source)
			}
			if tc.status == "" {
				if cost != nil {
					t.Fatal("9router got an XPiki price")
				}
				return
			}
			if cost.Status != tc.status || cost.TotalVND != tc.total {
				t.Fatalf("wrong cost: %+v", cost)
			}
		})
	}
}

func TestCustomerCostCacheFailureIsVisibleExpiresAndStaysBounded(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	credential := XPikiQuotaCredential{ChannelId: 8, APIKey: "test-failure-key", BaseURL: "https://api.xpiki.com"}
	makeReport := func() XPikiLogsReport {
		return XPikiLogsReport{Items: []XPikiLogItem{{LogID: 111, ChannelId: 8, Destination: "api.xpiki.com", CreatedAt: "2026-10-10T08:00:00+07:00", QuotaSource: "unmatched"}}}
	}
	report := makeReport()
	enrichCachedLocalAttemptCosts(context.Background(), &report, []XPikiQuotaCredential{credential}, server.URL)
	before := calls.Load()
	report = makeReport()
	enrichCachedLocalAttemptCosts(context.Background(), &report, []XPikiQuotaCredential{credential}, server.URL)
	if calls.Load() != before || len(report.QuotaErrors) == 0 || report.Items[0].TokenCost != nil {
		t.Fatal("cached failure hidden or treated as free")
	}
	localRequestCostCache.Lock()
	for key, entry := range localRequestCostCache.entries {
		entry.expires = time.Now().Add(-time.Second)
		localRequestCostCache.entries[key] = entry
	}
	localRequestCostCache.Unlock()
	report = makeReport()
	enrichCachedLocalAttemptCosts(context.Background(), &report, []XPikiQuotaCredential{credential}, server.URL)
	if calls.Load() <= before {
		t.Fatal("expired failure was never retried")
	}
	localRequestCostCache.Lock()
	count := len(localRequestCostCache.entries)
	localRequestCostCache.Unlock()
	if count > 2048 {
		t.Fatal("unbounded cost cache")
	}
}

func TestCustomerLogsLiveReadOnly(t *testing.T) {
	credentials := setupLiveXPikiReadOnly(t)
	report, err := FetchXPikiLogsReport(context.Background(), credentials, XPikiLogsFilter{Source: "newapi", GroupMode: "grouped", Date: "2026-10-10", PageSize: 100})
	if err != nil {
		t.Fatalf("live customer request query failed: %v", err)
	}
	if report.Source != "newapi" || len(report.Items) == 0 {
		t.Fatalf("live customer request report unavailable: source=%s rows=%d", report.Source, len(report.Items))
	}
	var ids []int
	for _, item := range report.Items {
		ids = append(ids, item.LogID)
	}
	var logs []model.Log
	if err := model.LOG_DB.Where("id IN ?", ids).Select("id, token_id, request_id, prompt_tokens, completion_tokens, other").Find(&logs).Error; err != nil {
		t.Fatal("cannot verify live request identities")
	}
	byID := make(map[int]model.Log)
	for _, log := range logs {
		byID[log.Id] = log
	}
	router, identified, endpoints := 0, 0, 0
	for _, item := range report.Items {
		log, ok := byID[item.LogID]
		if !ok || item.TokenID != log.TokenId || item.RequestID != log.RequestId || item.InputTokens != log.PromptTokens || item.OutputTokens != log.CompletionTokens {
			t.Fatal("live row changed customer key/request/token identity")
		}
		if item.TokenID > 0 {
			identified++
		}
		var metadata struct {
			RequestPath string `json:"request_path"`
		}
		if json.Unmarshal([]byte(log.Other), &metadata) == nil && metadata.RequestPath != "" {
			if item.RequestPath != metadata.RequestPath || item.Type == "unknown" {
				t.Fatal("live endpoint mislabelled")
			}
			endpoints++
		}
		if item.ChannelName == "9router" {
			router++
		}
	}
	if router == 0 {
		t.Fatal("9router requests missing from live NewAPI report")
	}
	var requestID string
	if err := model.LOG_DB.Raw(`SELECT request_id FROM logs WHERE type IN (2,5) AND request_id != '' GROUP BY request_id HAVING COUNT(*) > 1 ORDER BY MAX(id) DESC LIMIT 1`).Scan(&requestID).Error; err != nil {
		t.Fatal("cannot locate historical fallback")
	}
	if requestID != "" {
		fallback, err := fetchLocalRequestLogsReport(context.Background(), credentials, XPikiLogsFilter{Search: requestID})
		if err != nil || len(fallback.Items) != 1 || !fallback.Items[0].Retried || len(fallback.Items[0].Attempts) < 2 || len(fallback.Items[0].ChannelTrail) < 2 {
			t.Fatal("historical fallback missing from live customer report")
		}
	}
	t.Logf("LIVE_READ_ONLY_PASS: rows=%d exact_key_ids=%d recorded_endpoints=%d final_9router=%d fallback_verified=%t", len(report.Items), identified, endpoints, router, requestID != "")
}
