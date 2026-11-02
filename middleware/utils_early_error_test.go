package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMiddlewareEarlyAbortLogsExactlyOnce(t *testing.T) {
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
	oldLogType := common.LogDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = db, db, false, true
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = oldDB, oldLogDB, oldRedis, oldLogging
		common.SetLogDatabaseType(oldLogType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Log{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, setting TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	for i, status := range []int{400, 429, 503} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("id", 1)
		c.Set("token_id", 7)
		c.Set("token_name", "test-token")
		c.Set(common.RequestIdKey, "abort-request")
		if status == 503 {
			c.Set("group", "selected-group")
			c.Set("token_group", "auto")
		}
		abortWithOpenAiMessage(c, status, "early failure")
		if w.Code != status || !c.IsAborted() {
			t.Fatalf("abort changed response: %d", w.Code)
		}
		var count int64
		if err := db.Model(&model.Log{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != int64(i+1) {
			t.Fatalf("duplicate/missing abort log: %d", count)
		}
	}
	var row model.Log
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Type != model.LogTypeError || row.ChannelId != 0 || row.TokenId != 7 || row.Group != "default" || !strings.Contains(row.Other, `"admin_info"`) {
		t.Fatalf("wrong abort metadata: %+v", row)
	}
	var last model.Log
	if err := db.Last(&last).Error; err != nil {
		t.Fatal(err)
	}
	if last.Group != "selected-group" {
		t.Fatalf("lost resolved group: %s", last.Group)
	}
	for _, upstreamStatus := range []int{0, 429, 503} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("id", 1)
		err := types.NewOpenAIError(errors.New("last upstream error"), types.ErrorCodeBadResponseStatusCode, 400)
		err.UpstreamStatusCode = upstreamStatus
		RecordEarlyRelayError(c, "exhausted retry selection", false, err)
		last = model.Log{}
		if e := db.Last(&last).Error; e != nil {
			t.Fatal(e)
		}
		var metadata struct {
			Origin   string `json:"failure_origin"`
			Upstream int    `json:"upstream_status_code"`
			Final    bool   `json:"request_final"`
		}
		if e := json.Unmarshal([]byte(last.Other), &metadata); e != nil {
			t.Fatal(e)
		}
		wantOrigin := "unknown"
		if upstreamStatus > 0 {
			wantOrigin = "upstream"
		}
		if !metadata.Final || metadata.Origin != wantOrigin || metadata.Upstream != upstreamStatus {
			t.Fatalf("terminal selection lost last error provenance: %+v", metadata)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 1)
	c.Set("token_id", 7)
	c.Set("token_name", "test-token")
	c.Set("original_model", "gpt-6.1-sol")
	c.Set("group", "default")
	c.Set(common.RequestIdKey, "retry-exhausted")
	c.Set("use_channel", []string{"8", "11"})
	c.Set("channel_id", 11)
	RecordEarlyRelayError(c, "last upstream error", false)

	var adminLog model.Log
	if err := db.Where("request_id = ?", "retry-exhausted").First(&adminLog).Error; err != nil {
		t.Fatal(err)
	}
	if adminLog.ChannelId != 11 || !strings.Contains(adminLog.Other, "\"use_channel\":[\"8\",\"11\"]") {
		t.Fatalf("terminal retry log lost admin channel details: %+v", adminLog)
	}
	userLogs, total, err := model.GetUserLogs(1, model.LogTypeUnknown, 0, 0, "", "", 0, 10, "", "retry-exhausted", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(userLogs) != 1 || userLogs[0].ChannelId != 0 || strings.Contains(userLogs[0].Other, "admin_info") {
		t.Fatalf("terminal retry log exposed channel data to its owner: %+v", userLogs)
	}
}
