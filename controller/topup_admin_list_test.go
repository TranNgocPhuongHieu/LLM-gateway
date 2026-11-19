package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
)

func adminTopUpListContext(t *testing.T, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup?"+rawQuery, nil)
	return c, recorder
}

func TestParseAdminTopUpListQueryDefaultsToServerLocalDay(t *testing.T) {
	now := time.Date(2026, 11, 19, 15, 30, 0, 0, time.UTC)
	c, _ := adminTopUpListContext(t, "")
	query, err := parseAdminTopUpListQuery(c, now)
	if err != nil {
		t.Fatal(err)
	}
	dayStart := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC).Unix()
	if query.todayStart != dayStart || query.todayEnd != dayStart+86400 {
		t.Fatalf("today bounds = [%d, %d), want [%d, %d)", query.todayStart, query.todayEnd, dayStart, dayStart+86400)
	}
	if !reflect.DeepEqual(query.filter, model.AdminTopUpFilter{}) {
		t.Fatalf("default filter = %+v, want zero value", query.filter)
	}
}

func TestParseAdminTopUpListQueryMapsValidFilters(t *testing.T) {
	c, _ := adminTopUpListContext(t, "keyword=%20ORDER-7%20&status=needs_review&payment_method=binance"+
		"&start_timestamp=100&end_timestamp=200&today_start=300&today_end=400")
	query, err := parseAdminTopUpListQuery(c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	want := model.AdminTopUpFilter{Keyword: "ORDER-7", Status: model.AdminTopUpStatusNeedsReview,
		PaymentMethod: model.AdminTopUpMethodBinance, StartTime: 100, EndTime: 200}
	if !reflect.DeepEqual(query.filter, want) || query.todayStart != 300 || query.todayEnd != 400 {
		t.Fatalf("query = %+v, want filter %+v and today [300, 400)", query, want)
	}

	c, _ = adminTopUpListContext(t, "status=all&payment_method=all")
	query, err = parseAdminTopUpListQuery(c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(query.filter, model.AdminTopUpFilter{}) {
		t.Fatalf("'all' must mean no filter, got %+v", query.filter)
	}
}

func TestParseAdminTopUpListQueryRejectsInvalidParams(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"unknown status", "status=bogus"},
		{"unknown payment method", "payment_method=paypal"},
		{"keyword too long", "keyword=" + strings.Repeat("a", adminTopUpKeywordMaxLen+1)},
		{"non-numeric start", "start_timestamp=abc"},
		{"negative end", "end_timestamp=-5"},
		{"zero today start", "today_start=0&today_end=100"},
		{"end before start", "start_timestamp=200&end_timestamp=100"},
		{"today start without end", "today_start=300"},
		{"today end before start", "today_start=400&today_end=300"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := adminTopUpListContext(t, tc.query)
			if _, err := parseAdminTopUpListQuery(c, time.Now()); err == nil {
				t.Fatalf("query %q must be rejected", tc.query)
			}
		})
	}
}

func TestGetAllTopUpsRejectsInvalidFilterBeforeLoading(t *testing.T) {
	c, recorder := adminTopUpListContext(t, "status=bogus")
	GetAllTopUps(c)
	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Success || !strings.Contains(body.Message, "invalid status") {
		t.Fatalf("response = %+v, want success=false with invalid status message", body)
	}
}
