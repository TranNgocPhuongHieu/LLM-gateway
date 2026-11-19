package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func adminSummaryContext(t *testing.T, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/data/admin_summary?"+rawQuery, nil)
	return c, recorder
}

func setupAdminSummaryControllerDB(t *testing.T) {
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
	if err := db.AutoMigrate(&model.User{}, &model.ExternalIdentityClaim{}, &model.TopUp{},
		&model.SepayPayment{}, &model.BinanceTopUp{}, &model.BinanceDeposit{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
}

func TestParseAdminSummaryRangeRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct{ name, query, message string }{
		{"missing start", "end_timestamp=200", "invalid start_timestamp"},
		{"non-numeric start", "start_timestamp=abc&end_timestamp=200", "invalid start_timestamp"},
		{"zero start", "start_timestamp=0&end_timestamp=200", "invalid start_timestamp"},
		{"missing end", "start_timestamp=100", "invalid end_timestamp"},
		{"negative end", "start_timestamp=100&end_timestamp=-5", "invalid end_timestamp"},
		{"end before start", "start_timestamp=200&end_timestamp=100", "end_timestamp must not be before start_timestamp"},
		{"span over 366 days", "start_timestamp=1000&end_timestamp=" + strconv.FormatInt(1000+adminSummaryMaxSpanSeconds+1, 10), "time span cannot exceed 366 days"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := adminSummaryContext(t, tc.query)
			if _, _, ok := parseAdminSummaryRange(c); ok {
				t.Fatalf("query %q must be rejected", tc.query)
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			var body struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Success || body.Message != tc.message {
				t.Fatalf("body = %+v, want success=false message %q", body, tc.message)
			}
		})
	}
}

func TestParseAdminSummaryRangeAcceptsMaxSpan(t *testing.T) {
	c, _ := adminSummaryContext(t, "start_timestamp=1000&end_timestamp="+strconv.FormatInt(1000+adminSummaryMaxSpanSeconds, 10))
	if _, _, ok := parseAdminSummaryRange(c); !ok {
		t.Fatal("a span of exactly 366 days must be accepted")
	}
}

func TestGetAdminSummaryEmptyDatabaseEnvelope(t *testing.T) {
	setupAdminSummaryControllerDB(t)
	c, recorder := adminSummaryContext(t, "start_timestamp=1000&end_timestamp=5000")
	GetAdminSummary(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Topup struct {
				CreditedQuota *int64           `json:"credited_quota"`
				Count         *int64           `json:"count"`
				PendingCount  *int64           `json:"pending_count"`
				ByMethod      *json.RawMessage `json:"by_method"`
			} `json:"topup"`
			Users struct {
				NewUsers           *int64 `json:"new_users"`
				NewGoogleUsers     *int64 `json:"new_google_users"`
				SignupQuotaGranted *int64 `json:"signup_quota_granted"`
				ActiveUsers        *int64 `json:"active_users"`
			} `json:"users"`
			Requests struct {
				Total  *int64 `json:"total"`
				Errors *int64 `json:"errors"`
			} `json:"requests"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Success || body.Message != "" {
		t.Fatalf("envelope = %+v, want success=true message empty", body)
	}
	d := body.Data
	if d.Topup.CreditedQuota == nil || d.Topup.Count == nil || d.Topup.PendingCount == nil || d.Topup.ByMethod == nil ||
		d.Users.NewUsers == nil || d.Users.NewGoogleUsers == nil || d.Users.SignupQuotaGranted == nil || d.Users.ActiveUsers == nil ||
		d.Requests.Total == nil || d.Requests.Errors == nil {
		t.Fatalf("response is missing a contract field: %s", recorder.Body.String())
	}
	if string(*d.Topup.ByMethod) != "[]" {
		t.Fatalf("by_method = %s, want []", *d.Topup.ByMethod)
	}
}

func TestGetAdminErrorSummaryRejectsInvalidRangeAndReturnsEnvelope(t *testing.T) {
	c, recorder := adminSummaryContext(t, "start_timestamp=200&end_timestamp=100")
	GetAdminErrorSummary(c)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "must not be before") {
		t.Fatalf("status = %d, body = %s, want 400 for reversed range", recorder.Code, recorder.Body.String())
	}

	setupSelfUsageControllerDB(t)
	c, recorder = adminSummaryContext(t, "start_timestamp=1000&end_timestamp=5000")
	GetAdminErrorSummary(c)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Total      *int             `json:"total"`
			Truncated  *bool            `json:"truncated"`
			Categories *json.RawMessage `json:"categories"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || !body.Success || body.Data.Total == nil || body.Data.Truncated == nil ||
		body.Data.Categories == nil || string(*body.Data.Categories) != "[]" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
