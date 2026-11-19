package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSelfUsageControllerDB(t *testing.T) *gorm.DB {
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
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.MemoryCacheEnabled = oldMemoryCache
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Log{}, &model.QuotaData{}, &model.Token{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// callSelfUsageHandler runs a handler as user 1 and decodes the JSON envelope.
func callSelfUsageHandler(t *testing.T, handler gin.HandlerFunc, query string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	c.Set("id", 1)
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestUserUsageBreakdownEndpointValidation(t *testing.T) {
	setupSelfUsageControllerDB(t)
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{"missing start", "end_timestamp=100&group_by=model", "invalid start_timestamp"},
		{"missing end", "start_timestamp=1&group_by=model", "invalid end_timestamp"},
		{"end before start", "start_timestamp=200&end_timestamp=100&group_by=model", "invalid time range"},
		{"span too large", "start_timestamp=1&end_timestamp=2678402&group_by=model", "time span cannot exceed 31 days"},
		{"missing group_by", "start_timestamp=1&end_timestamp=100", "invalid group_by"},
		{"unknown group_by", "start_timestamp=1&end_timestamp=100&group_by=user", "invalid group_by"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := callSelfUsageHandler(t, GetUserUsageBreakdown, tc.query)
			if body["success"] != false || body["message"] != tc.want {
				t.Fatalf("want success=false message %q, got %v", tc.want, body)
			}
		})
	}
}

func TestUserUsageBreakdownEndpointReturnsOwnTotals(t *testing.T) {
	db := setupSelfUsageControllerDB(t)
	if err := db.Create(&[]model.QuotaData{
		{UserID: 1, ModelName: "gpt-4o", CreatedAt: 1000, Quota: 40, Count: 2, TokenUsed: 7},
		{UserID: 2, ModelName: "gpt-4o", CreatedAt: 1000, Quota: 999, Count: 5, TokenUsed: 5},
	}).Error; err != nil {
		t.Fatal(err)
	}
	body := callSelfUsageHandler(t, GetUserUsageBreakdown, "start_timestamp=1&end_timestamp=5000&group_by=model")
	if body["success"] != true {
		t.Fatalf("expected success: %v", body)
	}
	data := body["data"].(map[string]any)
	total := data["total"].(map[string]any)
	if total["quota"] != float64(40) || total["count"] != float64(2) || total["token_used"] != float64(7) {
		t.Fatalf("other user's usage leaked or totals wrong: %v", total)
	}
	items := data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["key"] != "gpt-4o" {
		t.Fatalf("unexpected items: %v", items)
	}
}

func TestUserErrorSummaryEndpointValidationAndResponse(t *testing.T) {
	db := setupSelfUsageControllerDB(t)

	body := callSelfUsageHandler(t, GetUserErrorSummary, "start_timestamp=1&end_timestamp=2678402")
	if body["success"] != false || body["message"] != "time span cannot exceed 31 days" {
		t.Fatalf("span limit not enforced: %v", body)
	}
	body = callSelfUsageHandler(t, GetUserErrorSummary, "end_timestamp=100")
	if body["success"] != false || body["message"] != "invalid start_timestamp" {
		t.Fatalf("missing start not rejected: %v", body)
	}

	rows := []model.Log{
		{UserId: 1, Type: model.LogTypeError, CreatedAt: 1000, ModelName: "gpt-4o", Content: "Unauthorized, invalid access token secret-user-data"},
		{UserId: 1, Type: model.LogTypeError, CreatedAt: 1100, ModelName: "claude", Content: "status_code=429, rate limit reached"},
		{UserId: 2, Type: model.LogTypeError, CreatedAt: 1100, ModelName: "gpt-4o", Content: "status_code=429, rate limit reached"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	body = callSelfUsageHandler(t, GetUserErrorSummary, "start_timestamp=1&end_timestamp=5000")
	if body["success"] != true {
		t.Fatalf("expected success: %v", body)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "secret-user-data") {
		t.Fatal("raw error content must never be returned")
	}
	data := body["data"].(map[string]any)
	if data["total"] != float64(2) || data["truncated"] != false {
		t.Fatalf("unexpected totals: %v", data)
	}
	categories := data["categories"].([]any)
	if len(categories) != 2 {
		t.Fatalf("unexpected categories: %v", categories)
	}
	first := categories[0].(map[string]any)
	if first["category"] != "rate_limited" || first["count"] != float64(1) || first["model_name"] != "claude" || first["last_at"] != float64(1100) {
		t.Fatalf("unexpected first category: %v", first)
	}
}
