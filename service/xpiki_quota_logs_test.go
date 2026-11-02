package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRequestLogPostgresPatternPreservesQueryArguments(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=fixture dbname=fixture sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	query := fmt.Sprintf(`SELECT substring(logs.other FROM '%s')::json FROM logs WHERE model_name = ? LIMIT ? OFFSET ?`, xpikiChannelTrailPattern)
	stmt := db.Raw(query, "fixture", 20, 0).Statement
	if stmt.Error != nil {
		t.Fatal(stmt.Error)
	}
	if !strings.Contains(stmt.SQL.String(), "WHERE model_name = $1 LIMIT $2 OFFSET $3") || len(stmt.Vars) != 3 {
		t.Fatalf("regex consumed query arguments: SQL=%s vars=%v", stmt.SQL.String(), stmt.Vars)
	}
	if strings.Contains(xpikiChannelTrailPattern, "?") {
		t.Fatal("GORM placeholder in regex literal")
	}
}

func setupXPikiRequestLogs(t *testing.T) *gorm.DB {
	t.Helper()
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
	channels := []model.Channel{{Id: 8, Name: "OpenAI-9001"}, {Id: 11, Name: "OpenAI-9303"}, {Id: 10, Name: "9router"}}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{Name: "unused-token", Key: "test-only"}).Error; err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 10, 8, 12, 0, 0, 0, time.Local).Unix()
	rows := []model.Log{
		{Id: 1, Type: 5, RequestId: "retry", ChannelId: 8, TokenName: "active", Content: "z-first-throttled", Username: "first-user"},
		{Id: 2, Type: 5, RequestId: "retry", ChannelId: 11, TokenName: "active", Content: "a-second-gateway", UpstreamRequestId: "upstream-failure"},
		{Id: 3, Type: 2, RequestId: "retry", ChannelId: 10, TokenName: "active", PromptTokens: 123, CompletionTokens: 4, Quota: 5},
		{Id: 4, Type: 5, RequestId: "early", ChannelId: 0, TokenName: "orphan", Content: "unexpected EOF"},
		{Id: 5, Type: 5, RequestId: "cancel", ChannelId: 14, TokenName: "retired", Content: "context canceled"},
		{Id: 6, Type: 2, ChannelId: 10},
		{Id: 7, Type: 5, ChannelId: 0, Content: "bad JSON"},
		{Id: 8, Type: 2, RequestId: "log_7", ChannelId: 10},
		{Id: 9, Type: 1, Content: "recharge"},
	}
	for i := range rows {
		rows[i].CreatedAt = stamp
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func requestLogs(t *testing.T, filter XPikiLogsFilter) XPikiLogsReport {
	t.Helper()
	report, err := fetchLocalRequestLogsReport(context.Background(), nil, filter)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestRequestLogsTerminalRowsRetainFallback(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	rows := []model.Log{
		{Id: 100, Type: 2, RequestId: "terminal-recovered", ChannelId: 11, ModelName: "fixture", Other: `{"admin_info":{"use_channel":["10","11"]}}`},
		{Id: 101, Type: 5, RequestId: "terminal-failed", ChannelId: 11, ModelName: "fixture", Content: "upstream unavailable", Other: `{"request_final":true,"admin_info":{"use_channel":["10","11"]}}`},
		{Id: 102, Type: 2, RequestId: "terminal-direct", ChannelId: 10, ModelName: "fixture", Other: `{"admin_info":{"use_channel":["10"]}}`},
		{Id: 103, Type: 2, RequestId: "terminal-invalid", ChannelId: 11, ModelName: "fixture", Other: `{"admin_info":{"use_channel":["invalid","11"]}}`},
		{Id: 104, Type: 2, RequestId: "terminal-broken", ChannelId: 11, ModelName: "fixture", Other: `{`},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	report := requestLogs(t, XPikiLogsFilter{Model: "fixture"})
	if report.Total != len(rows) {
		t.Fatalf("expected one row per request: %d", report.Total)
	}
	for _, item := range report.Items {
		switch item.RequestID {
		case "terminal-recovered", "terminal-failed":
			if !item.Retried || item.TotalAttempts != 2 || len(item.ChannelTrail) != 2 || item.InitialChannelId != 10 {
				t.Fatalf("missing terminal metadata trail: %+v", item)
			}
			if len(item.Attempts) != 0 {
				t.Fatal("must not invent attempt cost rows")
			}
			want, failed := "retry_recovered", 1
			if item.RequestID == "terminal-failed" {
				want, failed = "failed_final", 2
			}
			if item.RequestOutcome != want || item.FailedAttempts != failed {
				t.Fatalf("incorrect outcome: %+v", item)
			}
		default:
			if item.Retried {
				t.Fatalf("invalid or direct trail marked retried: %+v", item)
			}
		}
	}
	filtered := requestLogs(t, XPikiLogsFilter{Model: "fixture", Status: "retry_recovered"})
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].RequestID != "terminal-recovered" {
		t.Fatalf("recovered filter missed terminal metadata: %+v", filtered)
	}
}

func TestLocalRequestOutcomeRequiresTerminalEvidence(t *testing.T) {
	for _, tc := range []struct {
		name              string
		kind              int
		stream            bool
		content, metadata string
		failed            int
		want              string
	}{
		{"success", 2, false, "", "", 0, "success"},
		{"recovered", 2, false, "", "", 2, "retry_recovered"},
		{"attempt_only", 5, false, "unavailable", "", 1, "unknown"},
		{"final", 5, false, "unavailable", `{"request_final":true}`, 2, "failed_final"},
		{"cancel", 5, true, "context canceled", `{"request_final":true}`, 1, "cancelled"},
		{"upstream_text_not_client_cancel", 5, false, "provider aborted job", `{"request_final":true,"request_cancelled":false}`, 1, "failed_final"},
		{"client_cancelled", 5, false, "connection closed", `{"request_final":true,"request_cancelled":true}`, 1, "cancelled"},
		{"legacy_stream", 2, true, "", "", 0, "unknown"},
		{"stream_ok", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"done"}}`, 0, "success"},
		{"stream_eof_with_finish_reason", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"finish_reason:stop"}}`, 0, "success"},
		{"stream_eof_with_tool_calls", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"finish_reason:tool_calls"}}`, 0, "success"},
		{"stream_eof_with_length", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"finish_reason:length"}}`, 0, "success"},
		{"stream_eof_with_response_completed", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"event:response.completed"}}`, 0, "success"},
		{"stream_eof_without_evidence", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof"}}`, 0, "unknown"},
		{"stream_eof_recovered_with_evidence", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"finish_reason:stop"}}`, 1, "retry_recovered"},
		{"stream_response_incomplete", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"event:response.incomplete"}}`, 0, "stream_incomplete"},
		{"stream_incomplete_choices", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"incomplete_choices:1/2"}}`, 0, "stream_incomplete"},
		{"stream_response_failed", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"event:response.failed"}}`, 0, "stream_failed"},
		{"stream_event_error", 2, true, "", `{"stream_status":{"status":"ok","end_reason":"eof","terminal_evidence":"event:error"}}`, 0, "stream_failed"},
		{"stream_error", 2, true, "", `{"stream_status":{"status":"error"}}`, 0, "stream_failed"},
		{"stream_cancel", 2, true, "", `{"stream_status":{"status":"error","end_reason":"client_gone"}}`, 0, "cancelled"},
		{"invalid_metadata", 2, false, "", "{", 0, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := localRequestOutcome(tc.kind, tc.stream, tc.content, tc.metadata, tc.failed); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestLocalRequestHealthCohort(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	end := time.Date(2026, 10, 8, 12, 5, 0, 0, time.Local)
	for _, row := range []model.Log{
		{Id: 10, Type: 5, RequestId: "final", CreatedAt: end.Add(-time.Minute).Unix(), Other: `{"request_final":true,"status_code":429}`},
		{Id: 11, Type: 2, RequestId: "outside", CreatedAt: end.Unix()},
		{Id: 12, Type: 2, RequestId: "too-old", CreatedAt: end.Add(-16 * time.Minute).Unix()},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	r, err := GetLocalRequestHealth(context.Background(), end)
	if err != nil {
		t.Fatal(err)
	}
	if r.Attempts != 9 || r.Requests != 7 || r.Outcomes["retry_recovered"] != 1 || r.Outcomes["unknown"] != 4 || r.Outcomes["success"] != 1 || r.Outcomes["failed_final"] != 1 || r.Failures["http_429"] != 1 || len(r.Alerts) != 0 {
		t.Fatalf("unexpected cohort: %+v", r)
	}
}

func TestRequestHealthPreservesUpstreamProvenance(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	end := time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local)
	if err := db.Create(&model.Log{Type: 5, RequestId: "gateway-503", CreatedAt: end.Add(-time.Minute).Unix(), Other: `{"request_final":true,"failure_origin":"gateway","status_code":503}`}).Error; err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{429, 503} {
		resp := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture","type":"upstream_error"}}`))}
		e := RelayErrorHandler(context.Background(), resp, false)
		ResetStatusCode(e, fmt.Sprintf(`{"%d":400}`, status))
		if e.UpstreamStatusCode != status || e.StatusCode != 400 {
			t.Fatal("mapped status lost upstream provenance")
		}
		meta := fmt.Sprintf(`{"request_final":true,"status_code":400,"upstream_status_code":%d,"failure_origin":"upstream"}`, e.UpstreamStatusCode)
		if err := db.Create(&model.Log{Type: 5, RequestId: fmt.Sprint(status), CreatedAt: end.Add(-time.Minute).Unix(), Other: meta}).Error; err != nil {
			t.Fatal(err)
		}
	}
	r, err := GetLocalRequestHealth(context.Background(), end)
	if err != nil || r.Failures["upstream_429"] != 1 || r.Failures["upstream_5xx"] != 1 || r.Failures["gateway"] != 1 || r.Failures["http_5xx"] != 0 {
		t.Fatalf("wrong provenance: %+v %v", r, err)
	}
}

func TestLocalRequestHealthWarningRequiresKnownMinimum(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	end := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	for i := 0; i < 100; i++ {
		row := model.Log{Type: 5, RequestId: fmt.Sprintf("final-%d", i), CreatedAt: end.Add(-time.Minute).Unix(), Other: `{"request_final":true,"status_code":503}`}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	r, err := GetLocalRequestHealth(context.Background(), end)
	if err != nil || len(r.Alerts) != 1 || r.Failures["http_5xx"] != 100 {
		t.Fatalf("warning missing: %+v %v", r, err)
	}
	if err := db.Create(&model.Log{Type: 5, RequestId: "pending", CreatedAt: end.Add(-time.Minute).Unix()}).Error; err != nil {
		t.Fatal(err)
	}
	r, err = GetLocalRequestHealth(context.Background(), end)
	if err != nil || len(r.Alerts) != 0 || r.Outcomes["unknown"] != 1 {
		t.Fatalf("unknown cohort produced rate: %+v %v", r, err)
	}
}

func TestLocalRequestHealthCapAndCancellation(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	end := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	rows := make([]model.Log, 10001)
	for i := range rows {
		rows[i] = model.Log{Type: 5, RequestId: fmt.Sprintf("capped-%d", i), CreatedAt: end.Add(-time.Minute).Unix(), Other: `{"request_final":true}`}
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	r, err := GetLocalRequestHealth(context.Background(), end)
	if err != nil || !r.Capped || r.Attempts != 10000 || len(r.Alerts) != 0 {
		t.Fatalf("cap failed: %+v %v", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := GetLocalRequestHealth(ctx, end); err == nil || r != nil {
		t.Fatal("cancelled query returned fabricated data")
	}
}

func TestXPikiRequestLogsModesAndOrderedTrail(t *testing.T) {
	setupXPikiRequestLogs(t)
	report := requestLogs(t, XPikiLogsFilter{})
	if report.Total != 6 || len(report.Items) != 6 {
		t.Fatalf("grouped total=%d items=%d", report.Total, len(report.Items))
	}
	for _, item := range report.Items {
		if item.RequestID == "" && item.RequestOutcome != "unknown" {
			t.Fatal("missing request ID got a fabricated final outcome")
		}
		if item.RequestID != "retry" {
			continue
		}
		if item.LogID != 3 || item.Status != "success" || item.ChannelId != 10 || !item.Retried || item.TotalAttempts != 3 || item.FailedAttempts != 2 {
			t.Fatalf("wrong winner: %+v", item)
		}
		if item.RequestOutcome != "retry_recovered" {
			t.Fatal("retry outcome was not separated from attempts")
		}
		if item.InitialChannelId != 8 || item.InitialErrorCode != "z-first-throttled" {
			t.Fatalf("incorrect first error: %+v", item)
		}
		if !slices.Equal(item.ChannelTrail, []string{"OpenAI-9001 ✗", "OpenAI-9303 ✗", "9router ✓"}) {
			t.Fatalf("trail: %v", item.ChannelTrail)
		}
	}
	for _, mode := range []string{"all", "raw"} {
		report = requestLogs(t, XPikiLogsFilter{GroupMode: mode})
		if report.Total != 8 || len(report.Items) != 8 {
			t.Fatalf("%s total=%d items=%d", mode, report.Total, len(report.Items))
		}
		for i, item := range report.Items {
			if item.LogID != 8-i || item.Retried {
				t.Fatalf("unstable raw order/status: %+v", item)
			}
			if item.LogID == 2 && (item.AttemptNumber != 2 || item.Status != "failed") {
				t.Fatalf("wrong attempt: %+v", item)
			}
			if item.LogID == 4 && (item.ChannelName != "Chưa chọn kênh" || item.Destination != "") {
				t.Fatalf("fabricated channel: %+v", item)
			}
		}
	}
}

func TestCustomerRequestIdentityEndpointModelAndTotalTokens(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	for _, token := range []model.Token{{Id: 100, Name: "same-name", Key: "test-100"}, {Id: 101, Name: "same-name", Key: "test-101"}} {
		if err := db.Create(&token).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows := []model.Log{
		{Id: 20, Type: 5, RequestId: "identity", ChannelId: 8, TokenId: 100, ModelName: "gpt-6.1-sol", Content: "504 gateway timeout", Other: `{"request_path":"/v1/responses"}`},
		{Id: 21, Type: 2, RequestId: "identity", ChannelId: 10, TokenId: 100, ModelName: "gpt-6.1-sol", PromptTokens: 1000, CompletionTokens: 20, Other: `{"cache_tokens":800,"request_path":"/v1/responses"}`},
		{Id: 22, Type: 2, RequestId: "same-label-other-key", ChannelId: 10, TokenId: 101, TokenName: "same-name", ModelName: "gpt-5.6-sol", Other: `{"request_path":"/v1/chat/completions"}`},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	report := requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:100", Model: "gpt-6.1-sol"})
	if report.Total != 1 {
		t.Fatalf("wrong identity count: %d", report.Total)
	}
	item := report.Items[0]
	if item.TokenID != 100 || item.TokenName != "same-name" || item.RequestOrigin != "newapi" || item.RequestPath != "/v1/responses" || item.Type != "responses" {
		t.Fatalf("lost request identity/endpoint: %+v", item)
	}
	if item.InputTokens != 1000 || item.OutputTokens != 20 || item.CacheReadTokens != 800 {
		t.Fatalf("display input was reduced by cache: %+v", item)
	}
	if len(item.Attempts) != 2 || item.Attempts[0].Status != "failed" || item.Attempts[1].ChannelName != "9router" {
		t.Fatalf("lost fallback attempts: %+v", item)
	}
	if requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:101", Model: "gpt-6.1-sol"}).Total != 0 {
		t.Fatal("duplicate token names crossed identities")
	}
	chat := requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:101"}).Items[0]
	if chat.Type != "chat/completions" {
		t.Fatalf("wrong endpoint: %+v", chat)
	}
	if requestLogs(t, XPikiLogsFilter{Model: "' OR 1=1 --"}).Total != 0 {
		t.Fatal("model filter was not parameterized")
	}
	for _, value := range []string{"token_id:0", "token_id:invalid", "token_id:-1"} {
		if _, err := fetchLocalRequestLogsReport(context.Background(), nil, XPikiLogsFilter{KeyFilter: value}); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if !slices.Contains(report.ModelOptions, "gpt-5.6-sol") || !slices.Contains(report.ModelOptions, "gpt-6.1-sol") {
		t.Fatalf("lost model choices: %v", report.ModelOptions)
	}
}

func TestFailedFallbackDoesNotPretendToSucceedAndOldEndpointStaysUnknown(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	if err := db.Create(&[]model.Log{
		{Id: 20, Type: 5, RequestId: "all-failed", ChannelId: 10, Content: "504"},
		{Id: 21, Type: 5, RequestId: "all-failed", ChannelId: 8, Content: "503"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	item := requestLogs(t, XPikiLogsFilter{Search: "all-failed"}).Items[0]
	if item.Status != "failed" || !item.Retried || item.Type != "unknown" || !slices.Equal(item.ChannelTrail, []string{"9router ✗", "OpenAI-9001 ✗"}) {
		t.Fatalf("incorrect failed fallback: %+v", item)
	}
}

func TestRequestLogOptionsDoNotDuplicateResolvedTokenNames(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	if err := db.Create(&[]model.Token{
		{Id: 100, Name: "same-name", Key: "fixture-100"},
		{Id: 101, Name: "same-name", Key: "fixture-101"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Log{
		{Id: 100, Type: 2, TokenId: 100, TokenName: "same-name", ChannelId: 8},
		{Id: 101, Type: 2, TokenId: 101, TokenName: "same-name", ChannelId: 8},
		{Id: 102, Type: 2, TokenId: 100, TokenName: "old-name", ChannelId: 8},
	}).Error; err != nil {
		t.Fatal(err)
	}
	options := requestLogs(t, XPikiLogsFilter{}).KeyOptions
	values := make(map[string]int)
	for _, option := range options {
		values[option.Value]++
		if option.Value == "channel:14" {
			t.Fatalf("retired channel remains selectable: %+v", option)
		}
	}
	for _, value := range []string{"token_id:100", "token_id:101", "token_id:1"} {
		if values[value] != 1 {
			t.Fatalf("lost exact key identity %s: %v", value, values)
		}
	}
	for _, value := range []string{"token:same-name", "token:old-name", "token:unused-token"} {
		if values[value] != 0 {
			t.Fatalf("duplicate resolved name %s: %v", value, values)
		}
	}
	if requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:100"}).Total != 2 {
		t.Fatal("renamed key's old logs no longer match its exact ID")
	}
	// Legacy rows remain queryable, but must not add nonexistent API keys.
	if err := db.Create(&model.Log{Id: 103, Type: 2, TokenName: "same-name", ChannelId: 14}).Error; err != nil {
		t.Fatal(err)
	}
	foundLegacy := false
	for _, option := range requestLogs(t, XPikiLogsFilter{}).KeyOptions {
		if option.Value == "token:same-name" {
			foundLegacy = strings.HasPrefix(option.Label, "Khóa lịch sử")
		}
	}
	if foundLegacy {
		t.Fatal("unresolved legacy key remains selectable")
	}
	if requestLogs(t, XPikiLogsFilter{KeyFilter: "token:same-name"}).Total != 3 {
		t.Fatal("legacy filter compatibility was lost")
	}
	if requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:100", ChannelFilter: "channel:8"}).Total != 2 ||
		requestLogs(t, XPikiLogsFilter{KeyFilter: "token_id:100", ChannelFilter: "channel:10"}).Total != 0 {
		t.Fatal("independent key and channel filters were not intersected")
	}
	if err := db.Delete(&model.Token{}, 101).Error; err != nil {
		t.Fatal(err)
	}
	for _, option := range requestLogs(t, XPikiLogsFilter{}).KeyOptions {
		if option.Value == "token_id:101" {
			t.Fatal("deleted API key remains selectable")
		}
	}
}

func TestXPikiRequestLogsChannelAndSearchAcrossAttempts(t *testing.T) {
	setupXPikiRequestLogs(t)
	for _, filter := range []XPikiLogsFilter{
		{KeyFilter: "channel:8"}, {KeyFilter: "channel:11"}, {KeyFilter: "8"},
		{Search: "first-throttled"}, {Search: "first-user"}, {Search: "upstream-failure"}, {Search: "OpenAI-9001"},
	} {
		report := requestLogs(t, filter)
		if report.Total != 1 || report.Items[0].LogID != 3 {
			t.Fatalf("group filter %+v: %+v", filter, report)
		}
		filter.GroupMode = "all"
		report = requestLogs(t, filter)
		if report.Total != 1 || report.Items[0].Status != "failed" {
			t.Fatalf("raw filter %+v: %+v", filter, report)
		}
	}
	for _, mode := range []string{"grouped", "all"} {
		report := requestLogs(t, XPikiLogsFilter{GroupMode: mode, KeyFilter: "channel:0", Search: "EOF"})
		if report.Total != 1 || report.Items[0].LogID != 4 {
			t.Fatalf("early filter %s: %+v", mode, report)
		}
	}
}

func TestXPikiRequestLogsStatusAndPagination(t *testing.T) {
	setupXPikiRequestLogs(t)
	for _, tc := range []struct {
		mode, status string
		total        int
	}{
		{"grouped", "success", 3}, {"grouped", "retry_recovered", 1}, {"grouped", "failed", 2}, {"grouped", "canceled", 1},
		{"all", "success", 3}, {"all", "retry_recovered", 1}, {"all", "failed", 4}, {"all", "canceled", 1},
	} {
		report := requestLogs(t, XPikiLogsFilter{GroupMode: tc.mode, Status: tc.status})
		if report.Total != tc.total {
			t.Fatalf("%s/%s: %d != %d", tc.mode, tc.status, report.Total, tc.total)
		}
		if tc.status == "retry_recovered" {
			if len(report.Items) != 1 || report.Items[0].LogID != 3 || (tc.mode == "grouped" && report.Items[0].RequestOutcome != "retry_recovered") {
				t.Fatalf("%s recovered filter returned wrong request: %+v", tc.mode, report.Items)
			}
		}
	}
	for _, mode := range []string{"grouped", "all"} {
		first := requestLogs(t, XPikiLogsFilter{GroupMode: mode, PageSize: 2})
		next := requestLogs(t, XPikiLogsFilter{GroupMode: mode, PageSize: 2, Page: 2})
		empty := requestLogs(t, XPikiLogsFilter{GroupMode: mode, PageSize: 2, Page: 100})
		if !first.HasMore || len(next.Items) != 2 || empty.Total != first.Total || len(empty.Items) != 0 || empty.HasMore {
			t.Fatalf("pagination %s: %+v / %+v / %+v", mode, first, next, empty)
		}
		if first.Items[0].LogID == next.Items[0].LogID {
			t.Fatal("duplicate page")
		}
	}
}

func TestXPikiRequestLogsOptionsDatesAndValidation(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	report := requestLogs(t, XPikiLogsFilter{})
	var options []string
	for _, option := range report.KeyOptions {
		options = append(options, option.Value)
	}
	for _, want := range []string{"token_id:1", "channel:8", "channel:10", "channel:11"} {
		if !slices.Contains(options, want) {
			t.Fatalf("missing %s: %v", want, options)
		}
	}
	for _, unwanted := range []string{"token:active", "token:orphan", "token:retired", "channel:0", "channel:14"} {
		if slices.Contains(options, unwanted) {
			t.Fatalf("nonconfigured choice %s: %v", unwanted, options)
		}
	}
	boundary := time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local)
	if err := db.Create(&model.Log{Id: 10, Type: 2, CreatedAt: boundary.Unix(), RequestId: "next-day"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"grouped", "all"} {
		report := requestLogs(t, XPikiLogsFilter{GroupMode: mode, Date: "2026-10-08"})
		for _, item := range report.Items {
			if item.LogID == 10 {
				t.Fatal("end date included next midnight")
			}
		}
		if requestLogs(t, XPikiLogsFilter{GroupMode: mode, Date: "2026-10-09"}).Total != 1 {
			t.Fatal("start date excluded midnight")
		}
	}
	for _, filter := range []XPikiLogsFilter{{GroupMode: "invalid"}, {KeyFilter: "channel:invalid"}, {Date: "bad-date"}} {
		if _, err := fetchLocalRequestLogsReport(context.Background(), nil, filter); err == nil {
			t.Fatalf("accepted invalid filter: %+v", filter)
		}
	}
	if report := requestLogs(t, XPikiLogsFilter{Search: "' OR 1=1 --"}); report.Total != 0 {
		t.Fatal("search was not parameterized")
	}
	if err := db.Migrator().DropTable(&model.Token{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchLocalRequestLogsReport(context.Background(), nil, XPikiLogsFilter{}); err == nil || !strings.Contains(err.Error(), "load tokens") {
		t.Fatalf("token query failure hidden: %v", err)
	}
}

func TestXPikiRequestLogsPageSizesAreBounded(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	var rows []model.Log
	for id := 10; id <= 134; id++ {
		rows = append(rows, model.Log{Id: id, Type: model.LogTypeConsume, CreatedAt: 1791440000})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"grouped", "all"} {
		for _, size := range []int{20, 50, 100, 500} {
			report := requestLogs(t, XPikiLogsFilter{GroupMode: mode, PageSize: size})
			if len(report.Items) != min(size, 100) || !report.HasMore {
				t.Fatalf("%s page size %d: %d rows", mode, size, len(report.Items))
			}
		}
	}
}

func TestXPikiRequestLogsMultiUserFilterAndUsername(t *testing.T) {
	db := setupXPikiRequestLogs(t)
	if err := db.Create(&[]model.Log{
		{Id: 20, Type: 2, RequestId: "alice", ChannelId: 10, Username: "alice", CreatedAt: 1700000001},
		{Id: 21, Type: 2, RequestId: "bob", ChannelId: 10, Username: "bob", CreatedAt: 1700000002},
		{Id: 22, Type: 2, RequestId: "carol", ChannelId: 10, Username: "carol", CreatedAt: 1700000003},
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"grouped", "all"} {
		report := requestLogs(t, XPikiLogsFilter{GroupMode: mode, User: "alice, bob"})
		if report.Total != 2 {
			t.Fatalf("%s multi-user filter total = %d, want 2", mode, report.Total)
		}
		for _, item := range report.Items {
			if item.Username != "alice" && item.Username != "bob" {
				t.Fatalf("%s multi-user filter returned %q", mode, item.Username)
			}
		}
	}
	report := requestLogs(t, XPikiLogsFilter{GroupMode: "grouped", User: "alice"})
	if report.Total != 1 || report.Items[0].Username != "alice" {
		t.Fatalf("single user mapping: %+v", report)
	}
	if !slices.Contains(report.UserOptions, "alice") || !slices.Contains(report.UserOptions, "bob") ||
		!slices.Contains(report.UserOptions, "carol") {
		t.Fatalf("user options missing users: %v", report.UserOptions)
	}
}
