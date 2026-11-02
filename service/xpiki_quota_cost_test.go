package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func xpikiCostFixture() (XPikiLogItem, XPikiLogItem) {
	local := XPikiLogItem{
		CreatedAt: "2026-10-05T20:59:14+07:00", ModelSlug: "gpt-6.1-sol", Status: "success",
		InputTokens: 336402, OutputTokens: 292, CreditCost: 337862, ChannelId: 8,
		RequestID: "local-request", Destination: "api.xpiki.com", QuotaSource: "unmatched",
	}
	provider := local
	provider.CreditCost, provider.RequestID = 40497, "provider-request"
	provider.QuotaSource, provider.QuotaAstra, provider.UpstreamQuota = "xpiki", "202,485", "40497"
	return local, provider
}

func TestXPikiCostUsesProviderQuotaNotUserBill(t *testing.T) {
	local, provider := xpikiCostFixture()
	rows := []XPikiLogItem{local}
	attachMatchedXPikiCosts(rows, []XPikiLogItem{provider})
	got := rows[0]
	if got.QuotaAstra != "202,485" || got.UpstreamQuota != "40497" || got.QuotaSource != "xpiki" {
		t.Fatalf("wrong upstream cost: %+v", got)
	}
	if got.CreditCost != 337862 || got.InputTokens != 336402 || got.OutputTokens != 292 {
		t.Fatalf("user bill or input/output tokens changed: %+v", got)
	}
}

func TestXPikiCostMatchesObservedCachedRequests(t *testing.T) {
	// Values from confirmed XPiki ledger entries: input_tokens excludes cache.
	for _, tc := range []struct {
		name                           string
		input, uncached, cache, output int
		fee, astra                     string
	}{
		{"cached_45215", 45215, 24223, 20992, 631, "4449", "22,245"},
		{"cached_69798", 69798, 1702, 68096, 549, "4536", "22,680"},
		{"cached_69053", 69053, 36797, 32256, 464, "6558", "32,790"},
		{"cached_54322", 54322, 48690, 5632, 698, "6492", "32,460"},
		{"cached_32645", 32645, 27013, 5632, 527, "3816", "19,080"},
		{"cached_30686", 30686, 24926, 5760, 182, "3417", "17,085"},
		{"cached_334720", 334720, 3200, 331520, 217, "20370", "101,850"},
		{"cached_94211", 94211, 515, 93696, 117, "5736", "28,680"},
		{"fully_cached", 333824, 0, 333824, 57, "20055", "100,275"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, provider := xpikiCostFixture()
			local.InputTokens, local.OutputTokens, local.CacheReadTokens = tc.input, tc.output, tc.cache
			provider.InputTokens, provider.OutputTokens, provider.CacheReadTokens = tc.uncached, tc.output, tc.cache
			provider.UpstreamQuota, provider.QuotaAstra = tc.fee, tc.astra
			rows := []XPikiLogItem{local}
			attachMatchedXPikiCosts(rows, []XPikiLogItem{provider})
			if rows[0].QuotaSource != "xpiki" || rows[0].UpstreamQuota != tc.fee || rows[0].QuotaAstra != tc.astra {
				t.Fatalf("did not attach actual source fee: %+v", rows[0])
			}
			if rows[0].InputTokens != local.InputTokens || rows[0].OutputTokens != local.OutputTokens || rows[0].CacheReadTokens != local.CacheReadTokens || rows[0].CreditCost != local.CreditCost {
				t.Fatalf("local tokens/user billing were changed: %+v", rows[0])
			}
		})
	}
}

func TestXPikiCostCachedMatchesStayStrict(t *testing.T) {
	for _, scenario := range []string{"cache_write", "cache_mismatch", "input_mismatch", "inclusive_provider_input", "negative_cache", "cache_exceeds_input", "write_exceeds_input", "negative_input", "ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			local, provider := xpikiCostFixture()
			local.InputTokens, local.CacheReadTokens = 1000, 200
			provider.InputTokens, provider.CacheReadTokens = 800, 200
			providers := []XPikiLogItem{provider}
			switch scenario {
			case "cache_write":
				local.CacheWriteTokens, providers[0].CacheWriteTokens = 300, 300
				providers[0].InputTokens = 500
			case "cache_mismatch":
				providers[0].CacheReadTokens = 201
			case "input_mismatch":
				providers[0].InputTokens = 801
			case "inclusive_provider_input":
				providers[0].InputTokens = 1000
			case "negative_cache":
				local.CacheReadTokens, providers[0].CacheReadTokens = -1, -1
				providers[0].InputTokens = 1001
			case "cache_exceeds_input":
				local.CacheReadTokens, providers[0].CacheReadTokens = 1001, 1001
				providers[0].InputTokens = -1
			case "write_exceeds_input":
				local.CacheWriteTokens, providers[0].CacheWriteTokens = 801, 801
				providers[0].InputTokens = -1
			case "negative_input":
				local.InputTokens, providers[0].InputTokens = -1, -201
			case "ambiguous":
				other := provider
				other.RequestID = "another-provider-request"
				providers = append(providers, other)
			}
			rows := []XPikiLogItem{local}
			attachMatchedXPikiCosts(rows, providers)
			if scenario == "cache_write" {
				if rows[0].QuotaSource != "xpiki" || rows[0].QuotaAstra != provider.QuotaAstra {
					t.Fatalf("separate cache-write tokens did not match: %+v", rows[0])
				}
			} else if rows[0].QuotaSource != "unmatched" || rows[0].QuotaAstra != "" {
				t.Fatalf("guessed a cost from invalid/ambiguous cache usage: %+v", rows[0])
			}
		})
	}
}

func TestXPikiCostDoesNotGuessAmbiguousOrMismatchedRecords(t *testing.T) {
	local, provider := xpikiCostFixture()
	for _, scenario := range []string{"channel", "model", "time", "cache", "provider_id", "ambiguous", "duplicate_local", "zero_usage", "invalid_cost"} {
		t.Run(scenario, func(t *testing.T) {
			locals, providers := []XPikiLogItem{local}, []XPikiLogItem{provider}
			switch scenario {
			case "channel":
				providers[0].ChannelId = 12
			case "model":
				providers[0].ModelSlug = "gpt-6-astra"
			case "time":
				providers[0].CreatedAt = "2026-10-05T20:59:16+07:00"
			case "cache":
				providers[0].CacheReadTokens = 123
			case "provider_id":
				locals[0].UpstreamRequestID = "another-provider-request"
			case "ambiguous":
				other := provider
				other.RequestID = "second-provider-request"
				providers = append(providers, other)
			case "duplicate_local":
				other := local
				other.RequestID = "second-local-request"
				locals = append(locals, other)
			case "zero_usage":
				locals[0].InputTokens, locals[0].OutputTokens = 0, 0
				providers[0].InputTokens, providers[0].OutputTokens = 0, 0
			case "invalid_cost":
				providers[0].QuotaSource = "unavailable"
			}
			attachMatchedXPikiCosts(locals, providers)
			for _, got := range locals {
				if got.QuotaAstra != "" || got.QuotaSource != "unmatched" {
					t.Fatalf("invented a provider cost: %+v", got)
				}
			}
		})
	}
}

func TestXPikiCostPrefersExplicitUpstreamID(t *testing.T) {
	local, provider := xpikiCostFixture()
	local.UpstreamRequestID = provider.RequestID
	provider.ModelSlug = "mapped-upstream-model"
	provider.Status = "failed"
	provider.InputTokens, provider.OutputTokens = 0, 0
	rows := []XPikiLogItem{local}
	attachMatchedXPikiCosts(rows, []XPikiLogItem{provider})
	if rows[0].QuotaAstra != "202,485" {
		t.Fatalf("explicit provider ID was ignored: %+v", rows[0])
	}
}

func TestXPikiCostParsingUsesSameAstraMultiplierAsBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing provider authentication")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"items": []map[string]any{
				{"credit_cost": 40497}, {"credit_cost": 0}, {"credit_cost": "1.2"},
				{"credit_cost": -1}, {"credit_cost": "invalid"},
			}, "has_more": false,
		}})
	}))
	defer server.Close()
	items, _, err := fetchXPikiLogsForCredential(context.Background(), XPikiQuotaCredential{APIKey: "test-key"}, server.URL, 1, 50, "all", "all")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"202,485", "0", "6"} {
		if items[i].QuotaAstra != want || items[i].QuotaSource != "xpiki" {
			t.Fatalf("cost %d: %+v", i, items[i])
		}
	}
	for _, got := range items[3:] {
		if got.QuotaSource != "unavailable" || got.QuotaAstra != "" {
			t.Fatalf("invalid cost became a confirmed zero: %+v", got)
		}
	}
}

func xpikiPagedCostServer(t *testing.T, pageSize, total int) (*httptest.Server, *atomic.Int32, time.Time) {
	t.Helper()
	var calls atomic.Int32
	base := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			t.Error("invalid upstream page")
			page = 1
		}
		var items []map[string]any
		for i := (page - 1) * pageSize; i < min(page*pageSize, total); i++ {
			items = append(items, map[string]any{
				"created_at": base.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
				"model_slug": "gpt-6.1-sol", "type": "chat", "status": "success",
				"input_tokens": i + 1, "output_tokens": 292,
				"credit_cost": 40497, "request_id": "provider-" + strconv.Itoa(i),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"items": items, "has_more": page*pageSize < total,
		}})
	}))
	t.Cleanup(server.Close)
	return server, &calls, base
}

func TestXPikiCostHistoricalLookupIsBounded(t *testing.T) {
	server, calls, base := xpikiPagedCostServer(t, 20, 1000)
	stamp := base.Add(-501 * time.Minute)
	items, err := fetchXPikiCostRange(context.Background(), XPikiQuotaCredential{APIKey: "test-key", ChannelId: 8}, server.URL, stamp, stamp)
	if err != nil || len(items) != 1 || items[0].InputTokens != 502 {
		t.Fatalf("historical lookup: count=%d err=%v", len(items), err)
	}
	if calls.Load() > xpikiCostMaxPages {
		t.Fatalf("unbounded lookup: %d calls", calls.Load())
	}
}

func TestXPikiCostLookupStopsAtPageBudget(t *testing.T) {
	server, calls, base := xpikiPagedCostServer(t, 1, 1_000_000)
	stamp := base.Add(-900_000 * time.Minute)
	_, err := fetchXPikiCostRange(context.Background(), XPikiQuotaCredential{APIKey: "test-key"}, server.URL, stamp, stamp)
	if err == nil || calls.Load() > xpikiCostMaxPages {
		t.Fatalf("page budget not enforced: calls=%d err=%v", calls.Load(), err)
	}
}

func TestXPikiCostCancellationAndErrorsAreVisibleAndRedacted(t *testing.T) {
	local, _ := xpikiCostFixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "message": "Rejected test-secret-key", "data": nil})
	}))
	defer server.Close()
	for _, cancel := range []bool{false, true} {
		ctx, stop := context.WithCancel(context.Background())
		if cancel {
			stop()
		}
		report := XPikiLogsReport{Items: []XPikiLogItem{local}}
		enrichXPikiLogCosts(ctx, &report, []XPikiQuotaCredential{{ChannelId: 8, APIKey: "test-secret-key"}}, server.URL)
		stop()
		if len(report.QuotaErrors) == 0 || report.Items[0].QuotaAstra != "" || strings.Contains(strings.Join(report.QuotaErrors, " "), "test-secret-key") {
			t.Fatalf("hidden error or leaked key: %+v", report)
		}
	}
}

func TestXPikiCostDoesNotChargeFallbackAsXPiki(t *testing.T) {
	local, _ := xpikiCostFixture()
	local.ChannelId, local.Destination = 10, "9router.trnhieu.dev"
	report := XPikiLogsReport{Items: []XPikiLogItem{local}}
	enrichXPikiLogCosts(context.Background(), &report, nil, "http://unused.invalid")
	if report.Items[0].QuotaSource != "not_xpiki" || report.Items[0].QuotaAstra != "" {
		t.Fatalf("9router user bill was converted into XPiki cost: %+v", report.Items[0])
	}
}

func TestXPikiCostLocalReportKeepsInputOutputAndAttachesProviderCost(t *testing.T) {
	for _, cacheTokens := range []int{0, 20992, 336402} {
		t.Run("cache_"+strconv.Itoa(cacheTokens), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			conn.SetMaxOpenConns(1)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			model.DB, model.LOG_DB = db, db
			t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB; _ = conn.Close() })
			if err := db.AutoMigrate(&model.Log{}, &model.Channel{}, &model.Token{}); err != nil {
				t.Fatal(err)
			}
			stamp, _ := time.Parse(time.RFC3339, "2026-10-05T20:59:14+07:00")
			baseURL := "https://api.xpiki.com"
			if err := db.Create(&model.Channel{Id: 8, Name: "OpenAI-9001", BaseURL: &baseURL}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Log{Id: 12144, Type: 2, CreatedAt: stamp.Unix(), RequestId: "local-request",
				ModelName: "gpt-6.1-sol", ChannelId: 8, TokenName: "vilao", Quota: 337862,
				PromptTokens: 336402, CompletionTokens: 292, Other: "{\"cache_tokens\":" + strconv.Itoa(cacheTokens) + "}"}).Error; err != nil {
				t.Fatal(err)
			}
			cred := XPikiQuotaCredential{ChannelId: 8, APIKey: "test-key", BaseURL: baseURL}
			report, err := fetchLocalRequestLogsReport(context.Background(), []XPikiQuotaCredential{cred}, XPikiLogsFilter{Page: 1, PageSize: 20})
			if err != nil || len(report.Items) != 1 {
				t.Fatalf("local report: count=%d err=%v", len(report.Items), err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
					"has_more": false, "items": []map[string]any{{"created_at": stamp.Format(time.RFC3339),
						"model_slug": "gpt-6.1-sol", "status": "success", "input_tokens": 336402 - cacheTokens,
						"cache_read_tokens": cacheTokens,
						"output_tokens":     292, "credit_cost": 40497, "request_id": "provider-request"}},
				}})
			}))
			defer server.Close()
			enrichXPikiLogCosts(context.Background(), &report, []XPikiQuotaCredential{cred}, server.URL)
			got := report.Items[0]
			if got.InputTokens != 336402 || got.OutputTokens != 292 || got.CacheReadTokens != cacheTokens || got.QuotaAstra != "202,485" || got.CreditCost != 337862 {
				t.Fatalf("wrong local/provider projection: %+v; errors=%v", got, report.QuotaErrors)
			}
		})
	}
}
