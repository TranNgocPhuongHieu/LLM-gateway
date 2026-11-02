package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ledgerFixture(t *testing.T, sourceCount, total int, step time.Duration) (*httptest.Server, []XPikiQuotaCredential, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var source int
		if _, err := fmt.Sscanf(key, "ledger-test-key-%d", &source); err != nil {
			w.WriteHeader(401)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if page < 1 || size < 1 || size > 50 {
			w.WriteHeader(400)
			return
		}
		var all []map[string]any
		for i := 0; i < total; i++ {
			all = append(all, map[string]any{"created_at": base.Add(-time.Duration(i*sourceCount+source) * step).Format(time.RFC3339), "type": "chat", "model_slug": "gpt-6.1-sol", "status": "success", "input_tokens": 100, "output_tokens": 3, "credit_cost": 600000, "request_id": fmt.Sprintf("source-%d-row-%d", source, i)})
		}
		start := min((page-1)*size, len(all))
		end := min(start+size, len(all))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": all[start:end], "has_more": end < len(all)}})
	}))
	t.Cleanup(server.Close)
	var credentials []XPikiQuotaCredential
	for i := 0; i < sourceCount; i++ {
		credentials = append(credentials, XPikiQuotaCredential{APIKey: fmt.Sprintf("ledger-test-key-%d", i), ChannelId: 8 + i, ChannelName: fmt.Sprintf("Source %d", i), BaseURL: "https://api.xpiki.com"})
	}
	return server, credentials, &calls
}

// Opt-in production read-only reconciliation. No relay/model requests, database
// writes, or credentials in fixtures/output; every connection enforces read-only.
func setupLiveXPikiReadOnly(t *testing.T) []XPikiQuotaCredential {
	t.Helper()
	if os.Getenv("VTDM_XPIKI_LIVE_READ_ONLY") != "1" {
		t.Skip("requires explicit live read-only validation")
	}
	inspect := func(name string) []byte {
		out, err := exec.Command("docker", "inspect", name).Output()
		if err != nil {
			t.Fatal("cannot inspect live service metadata")
		}
		return out
	}
	var runtime []struct{ Config struct{ Env []string } }
	if err := json.Unmarshal(inspect("new-api"), &runtime); err != nil || len(runtime) != 1 {
		t.Fatal("invalid runtime metadata")
	}
	var dsn string
	for _, entry := range runtime[0].Config.Env {
		if strings.HasPrefix(entry, "SQL_DSN=") {
			dsn = strings.TrimPrefix(entry, "SQL_DSN=")
		}
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "postgres" {
		t.Fatal("unexpected database target")
	}
	var database []struct {
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if err := json.Unmarshal(inspect("new-api-postgres"), &database); err != nil || len(database) != 1 {
		t.Fatal("invalid database metadata")
	}
	ip := database[0].NetworkSettings.Networks["newapi_default"].IPAddress
	if net.ParseIP(ip) == nil {
		t.Fatal("cannot identify database address")
	}
	u.Host = net.JoinHostPort(ip, "5432")
	query := u.Query()
	query.Set("options", "-c default_transaction_read_only=on -c statement_timeout=5000")
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open read-only database connection")
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal("cannot obtain read-only database connection")
	}
	t.Cleanup(func() { _ = conn.Close() })
	var readOnly string
	if err := db.Raw("SHOW default_transaction_read_only").Scan(&readOnly).Error; err != nil || readOnly != "on" {
		t.Fatal("read-only database guard not active")
	}
	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB })
	var channels []model.Channel
	if err := db.Find(&channels).Error; err != nil {
		t.Fatal("cannot read configured channels")
	}
	var credentials []XPikiQuotaCredential
	for _, ch := range channels {
		if !IsXPikiBaseURL(ch.GetBaseURL()) {
			continue
		}
		for _, key := range ch.GetKeys() {
			credentials = append(credentials, XPikiQuotaCredential{APIKey: strings.TrimSpace(key), ChannelId: ch.Id, ChannelName: ch.Name, BaseURL: ch.GetBaseURL()})
		}
	}
	return credentials
}

func TestXPikiLedgerLiveReadOnly(t *testing.T) {
	credentials := setupLiveXPikiReadOnly(t)
	for _, filter := range []XPikiLogsFilter{{PageSize: 100}, {Page: 3, PageSize: 100}, {KeyFilter: "channel:12", Date: "2026-10-09", PageSize: 100},
		{KeyFilter: "channel:12", Date: "2026-10-09", Search: "3b748cfd-c25d-4091-b990-85a2a6ba85bf", PageSize: 100}} {
		report, err := fetchXPikiLogsReportAtURL(context.Background(), credentials, XPikiKeyLogsURL, filter)
		wantRows := 100
		if filter.Search != "" {
			wantRows = 1
		}
		if err != nil || report.Incomplete || len(report.Items) != wantRows || len(report.KeyOptions) != 6 {
			t.Fatalf("live ledger report incomplete: rows=%d sources=%d warnings=%v", len(report.Items), len(report.KeyOptions)-1, report.QuotaErrors)
		}
		var matched, unmatched int
		foundMissing := false
		for _, item := range report.Items {
			if item.RequestOrigin == "newapi" {
				matched++
			} else {
				unmatched++
			}
			if item.RequestID == "3b748cfd-c25d-4091-b990-85a2a6ba85bf" {
				foundMissing = true
				if item.RequestOrigin != "unmatched" {
					t.Fatal("provider-only record incorrectly attributed")
				}
			}
		}
		if filter.Search != "" && !foundMissing {
			t.Fatal("known missing provider request was not restored to report")
		}
		t.Logf("READ_ONLY_PASS page=%d size=%d rows=%d distinct_keys=%d matched_newapi=%d unmatched=%d provider_only_example_found=%t", max(filter.Page, 1), filter.PageSize, len(report.Items), len(report.KeyOptions)-1, matched, unmatched, foundMissing)
	}
}

func TestXPikiLedgerReadsBeyondFirstFiftyAndPreservesSources(t *testing.T) {
	server, creds, calls := ledgerFixture(t, 5, 90, time.Second)
	shared := creds[4]
	shared.ChannelId = 14
	shared.ChannelName = "Shared alias"
	creds = append(creds, shared, XPikiQuotaCredential{APIKey: "not-xpiki", ChannelId: 10, BaseURL: "https://9router.example"})
	for _, tc := range []struct {
		page, size, want int
		first            string
		total            int
		more             bool
	}{
		{1, 20, 20, "source-0-row-0", -1, true}, {3, 100, 100, "source-0-row-40", -1, true}, {5, 100, 50, "source-0-row-80", 450, false},
	} {
		report, err := fetchXPikiLogsReportAtURL(context.Background(), creds, server.URL, XPikiLogsFilter{Page: tc.page, PageSize: tc.size})
		if err != nil {
			t.Fatal(err)
		}
		if report.Incomplete || len(report.Items) != tc.want || report.Items[0].RequestID != tc.first || report.Total != tc.total || report.HasMore != tc.more {
			t.Fatalf("page %+v: %+v", tc, report)
		}
		if len(report.KeyOptions) != 6 {
			t.Fatalf("must have all + 5 distinct keys, not duplicate/shared/9router: %+v", report.KeyOptions)
		}
		for _, item := range report.Items {
			if item.CostVND != "3,120 ₫" || item.RequestOrigin != "unmatched" || strings.Contains(item.LogKey, "ledger-test-key") {
				t.Fatalf("cost/source leaked or guessed: %+v", item)
			}
			if strings.HasPrefix(item.RequestID, "source-4-") && (len(item.SourceChannels) != 2 || item.ChannelId != 0 || !strings.Contains(item.ChannelName, "Shared alias")) {
				t.Fatalf("lost shared-key alias: %+v", item)
			}
		}
	}
	if calls.Load() <= 15 {
		t.Fatalf("did not fetch additional upstream pages: %d", calls.Load())
	}
}

func TestXPikiLedgerSearchAndDateSeekReachOlderPages(t *testing.T) {
	server, creds, calls := ledgerFixture(t, 1, 5000, time.Minute)
	report, err := fetchXPikiLogsReportAtURL(context.Background(), creds, server.URL, XPikiLogsFilter{Date: "2026-10-08", PageSize: 20})
	if err != nil || report.Incomplete || len(report.Items) != 20 || report.Items[0].RequestID != "source-0-row-721" {
		t.Fatalf("older date seek: %+v err=%v", report, err)
	}
	if calls.Load() > 20 {
		t.Fatalf("date window scanned every newer page: %d", calls.Load())
	}
	server2, creds2, _ := ledgerFixture(t, 1, 160, time.Second)
	report, err = fetchXPikiLogsReportAtURL(context.Background(), creds2, server2.URL, XPikiLogsFilter{Search: "source-0-row-120"})
	if err != nil || report.Incomplete || len(report.Items) != 1 || report.Total != 1 || report.HasMore {
		t.Fatalf("search after first page: %+v err=%v", report, err)
	}
	report, err = fetchXPikiLogsReportAtURL(context.Background(), creds2, server2.URL, XPikiLogsFilter{KeyFilter: "missing-key"})
	if err != nil || report.Total != 0 || len(report.Items) != 0 {
		t.Fatalf("invalid source filter fell back to all keys: %+v %v", report, err)
	}
}

func TestXPikiLedgerPartialFailureIsVisibleAndDoesNotHideOtherKeys(t *testing.T) {
	server, creds, _ := ledgerFixture(t, 1, 60, time.Second)
	creds = append(creds, XPikiQuotaCredential{APIKey: "test-secret-invalid", ChannelId: 9, ChannelName: "Unavailable", BaseURL: "https://api.xpiki.com"})
	report, err := fetchXPikiLogsReportAtURL(context.Background(), creds, server.URL, XPikiLogsFilter{PageSize: 100})
	if err != nil || !report.Incomplete || len(report.Items) != 60 || report.Total != -1 || len(report.QuotaErrors) != 1 || strings.Contains(strings.Join(report.QuotaErrors, " "), "test-secret-invalid") {
		t.Fatalf("hidden partial failure or secret: %+v %v", report, err)
	}
	server2, creds2, calls := ledgerFixture(t, 1, 2500, time.Second)
	report, err = fetchXPikiLogsReportAtURL(context.Background(), creds2, server2.URL, XPikiLogsFilter{Search: "does-not-exist"})
	if err != nil || !report.Incomplete || report.Total != -1 || !report.HasMore || calls.Load() > 40 {
		t.Fatalf("scan budget presented as complete/empty: %+v %v calls=%d", report, err, calls.Load())
	}
}

func TestXPikiLedgerOriginUsesCorrectSharedChannelAndRejectsStaleFallbackID(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	logs := []model.Log{
		{Id: 20, Type: 2, ChannelId: 14, RequestId: "shop-request", UpstreamRequestId: "provider-shared", TokenName: "shop-a"},
		{Id: 21, Type: 2, ChannelId: 10, RequestId: "fallback-request", UpstreamRequestId: "provider-shared", TokenName: "wrong-stale-token"},
		{Id: 22, Type: 2, ChannelId: 13, RequestId: "duplicate-a", UpstreamRequestId: "ambiguous", TokenName: "shop-a"},
		{Id: 23, Type: 2, ChannelId: 14, RequestId: "duplicate-b", UpstreamRequestId: "ambiguous", TokenName: "shop-b"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	items := []XPikiLogItem{
		{RequestID: "provider-shared", SourceChannels: []XPikiSourceChannel{{ID: 13, Name: "Claude"}, {ID: 14, Name: "Opus"}}, RequestOrigin: "unmatched"},
		{RequestID: "ambiguous", SourceChannels: []XPikiSourceChannel{{ID: 13, Name: "Claude"}, {ID: 14, Name: "Opus"}}, RequestOrigin: "unmatched"},
		{RequestID: "provider-only", SourceChannels: []XPikiSourceChannel{{ID: 8, Name: "Source 8"}}, RequestOrigin: "unmatched"},
	}
	if err := attachXPikiRequestOrigins(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].RequestOrigin != "newapi" || items[0].OriginMatch != "request_id" || items[0].ChannelId != 14 || items[0].TokenName != "shop-a" || items[0].LocalRequestID != "shop-request" {
		t.Fatalf("incorrect origin attribution: %+v", items[0])
	}
	if items[1].RequestOrigin != "ambiguous" || items[1].TokenName != "" {
		t.Fatalf("guessed ambiguous source: %+v", items[1])
	}
	if items[2].RequestOrigin != "unmatched" || items[2].TokenName != "" {
		t.Fatalf("unmatched is not proof of direct provider traffic: %+v", items[2])
	}
}

func TestXPikiLedgerMissingFeeDoesNotBecomeConfirmedZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"has_more": false, "items": []map[string]any{
			{"request_id": "missing", "credit_cost": nil}, {"request_id": "zero", "credit_cost": 0},
		}}})
	}))
	defer server.Close()
	rows, _, err := fetchXPikiLogsForCredential(context.Background(), XPikiQuotaCredential{APIKey: "test-only"}, server.URL, 1, 50, "all", "all")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].QuotaSource != "unavailable" || rows[0].CostVND != "" || rows[0].UpstreamQuota != "" {
		t.Fatalf("missing fee was shown as zero: %+v", rows[0])
	}
	if rows[1].QuotaSource != "xpiki" || rows[1].CostVND != "0 ₫" || rows[1].UpstreamQuota != "0" {
		t.Fatalf("explicit zero lost: %+v", rows[1])
	}
}

func TestXPikiLedgerFailedAttemptShowsVerified9routerCompletion(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	stamp := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, ch := range []model.Channel{{Id: 8, Name: "XPiki"}, {Id: 10, Name: "9router"}} {
		if err := db.Save(&ch).Error; err != nil {
			t.Fatal(err)
		}
	}
	logs := []model.Log{
		{Id: 100, Type: model.LogTypeError, ChannelId: 8, RequestId: "fallback-request", UpstreamRequestId: "xpiki-error", CreatedAt: stamp.Unix()},
		{Id: 101, Type: model.LogTypeConsume, ChannelId: 10, RequestId: "fallback-request", CreatedAt: stamp.Add(time.Second).Unix()},
		{Id: 102, Type: model.LogTypeError, ChannelId: 8, RequestId: "client-left", UpstreamRequestId: "xpiki-canceled", CreatedAt: stamp.Unix()},
		{Id: 103, Type: model.LogTypeConsume, ChannelId: 10, RequestId: "different-request", CreatedAt: stamp.Add(time.Second).Unix()},
		{Id: 104, Type: model.LogTypeConsume, ChannelId: 10, RequestId: "older-completion", CreatedAt: stamp.Unix()},
		{Id: 105, Type: model.LogTypeError, ChannelId: 8, RequestId: "older-completion", UpstreamRequestId: "xpiki-after-completion", CreatedAt: stamp.Add(time.Second).Unix()},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	var items []XPikiLogItem
	for _, id := range []string{"xpiki-error", "xpiki-canceled", "xpiki-after-completion", "unmatched-provider"} {
		items = append(items, XPikiLogItem{RequestID: id, Status: "failed", ChannelId: 8, Destination: "api.xpiki.com",
			SourceChannels: []XPikiSourceChannel{{ID: 8, Name: "XPiki"}}, QuotaSource: "xpiki", UpstreamQuota: "0", CostVND: "0 ₫",
			CreatedAt: stamp.Format(time.RFC3339)})
	}
	if err := attachXPikiRequestOrigins(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if got := items[0]; got.LocalFinalStatus != "success" || got.FallbackChannelID != 10 || got.FallbackChannelName != "9router" {
		t.Fatalf("real fallback completion hidden: %+v", got)
	}
	if items[0].Status != "failed" || items[0].ChannelId != 8 || items[0].Destination != "api.xpiki.com" || items[0].CostVND != "0 ₫" {
		t.Fatal("fallback metadata replaced the provider attempt or its fee")
	}
	for _, item := range items[1:] {
		if item.LocalFinalStatus != "" || item.FallbackChannelID != 0 {
			t.Fatalf("guessed fallback from an unrelated or older completion: %+v", item)
		}
	}
}

func TestXPikiLedgerLocalOriginFailureDoesNotFilterProviderRows(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	if err := db.Migrator().DropTable(&model.Log{}); err != nil {
		t.Fatal(err)
	}
	server, creds, _ := ledgerFixture(t, 1, 80, time.Second)
	report, err := fetchXPikiLogsReportAtURL(context.Background(), creds, server.URL, XPikiLogsFilter{PageSize: 100})
	if err != nil || len(report.Items) != 80 || report.Total != 80 || report.Incomplete || len(report.QuotaErrors) == 0 {
		t.Fatalf("local DB failure hid provider history: %+v %v", report, err)
	}
	for _, row := range report.Items {
		if row.RequestOrigin != "unmatched" {
			t.Fatalf("guessed origin after DB failure: %+v", row)
		}
	}
}

func TestXPikiLedgerLegacyOriginIsNotPresentedAsExact(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	stamp := time.Date(2026, 10, 9, 8, 0, 0, 0, time.Local)
	if err := db.Create(&model.Log{Id: 30, Type: 2, ChannelId: 8, CreatedAt: stamp.Unix(), ModelName: "gpt-6.1-sol", PromptTokens: 1000, CompletionTokens: 10, Other: `{"cache_tokens":200}`, RequestId: "legacy-request", TokenName: "legacy-token"}).Error; err != nil {
		t.Fatal(err)
	}
	items := []XPikiLogItem{{RequestID: "legacy-provider", CreatedAt: stamp.Format(time.RFC3339), Status: "success", ModelSlug: "gpt-6.1-sol", InputTokens: 800, OutputTokens: 10, CacheReadTokens: 200, QuotaSource: "xpiki", SourceChannels: []XPikiSourceChannel{{ID: 8, Name: "Source 8"}}, RequestOrigin: "unmatched"}}
	if err := attachXPikiRequestOrigins(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].RequestOrigin != "usage_match" || items[0].OriginMatch != "usage_time" || items[0].TokenName != "legacy-token" {
		t.Fatalf("legacy match hidden or incorrectly marked exact: %+v", items[0])
	}
}
