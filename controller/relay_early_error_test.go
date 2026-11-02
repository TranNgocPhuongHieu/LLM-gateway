package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRelayEarlyParseErrorRecordedBeforeRelayInfoExists(t *testing.T) {
	for _, tc := range []struct {
		name             string
		enabled          bool
		userID, wantLogs int
		readError        bool
	}{
		{"authenticated", true, 1, 1, false}, {"logging-disabled", false, 1, 0, false}, {"unauthenticated", true, 0, 0, false},
		{"unexpected-eof", true, 1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			conn.SetMaxOpenConns(1)
			oldDB, oldLogDB, oldRedis, oldLogging := model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled
			model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = db, db, false, tc.enabled
			t.Cleanup(func() {
				model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = oldDB, oldLogDB, oldRedis, oldLogging
				_ = conn.Close()
			})
			if err := db.AutoMigrate(&model.Log{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, setting TEXT)").Error; err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-6.1-sol",`))
			expectedParseError := "unexpected end of JSON input"
			if tc.readError {
				c.Request.Body = io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF))
				expectedParseError = "unexpected EOF"
			}
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", tc.userID)
			c.Set("token_id", 7)
			c.Set("token_name", "test-token")
			c.Set("original_model", "gpt-6.1-sol")
			c.Set(common.RequestIdKey, "parse-error-request")
			activeBefore := perfmetrics.ActiveRelayRequests.Load()
			Relay(c, types.RelayFormatOpenAI)
			if perfmetrics.ActiveRelayRequests.Load() != activeBefore {
				t.Fatal("early relay failure leaked active request gauge")
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("wrong status: %d", w.Code)
			}
			var logs []model.Log
			if err := db.Find(&logs).Error; err != nil {
				t.Fatal(err)
			}
			if len(logs) != tc.wantLogs {
				t.Fatalf("got %d logs, want %d", len(logs), tc.wantLogs)
			}
			if len(logs) == 1 {
				row := logs[0]
				if row.Type != model.LogTypeError || row.ChannelId != 0 || row.UserId != 1 || row.TokenId != 7 || row.TokenName != "test-token" || row.RequestId != "parse-error-request" || !strings.Contains(row.Content, expectedParseError) || !strings.Contains(row.Other, "request_path") {
					t.Fatalf("incomplete early error: %+v", row)
				}
				var metadata struct {
					Final  bool   `json:"request_final"`
					Origin string `json:"failure_origin"`
					Status int    `json:"status_code"`
				}
				if err := json.Unmarshal([]byte(row.Other), &metadata); err != nil || !metadata.Final || metadata.Origin != "gateway" || metadata.Status != 400 {
					t.Fatal("early parse error lost terminal provenance")
				}
			}
		})
	}
}

func TestXPikiLogsRejectsInvalidRequestBeforeLoadingCredentials(t *testing.T) {
	for _, body := range []string{`{"page":`, `{"group_mode":"invalid"}`, `{"page_size":"bad"}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/xpiki_quota/logs", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		CheckXPikiLogs(c)
		if !strings.Contains(w.Body.String(), `"success":false`) {
			t.Fatalf("invalid body accepted: %s", w.Body.String())
		}
	}
}
