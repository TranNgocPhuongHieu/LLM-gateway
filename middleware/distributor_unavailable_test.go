package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/constant"
	"github.com/TranNgocPhuongHieu/LLM-gateway/i18n"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAbortNoChannelSeparatesRemovedModelsFromTransientFailures(t *testing.T) {
	if err := i18n.Init(); err != nil {
		t.Fatal(err)
	}
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
	oldLogType, oldAddress := common.LogDatabaseType(), system_setting.ServerAddress
	model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = db, db, false, true
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	system_setting.ServerAddress = "https://gw.example.test/"
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, constant.ErrorLogEnabled = oldDB, oldLogDB, oldRedis, oldLogging
		common.SetLogDatabaseType(oldLogType)
		system_setting.ServerAddress = oldAddress
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Log{}, &model.Channel{}, &model.Ability{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, setting TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Channel{Id: 1, Status: common.ChannelStatusAutoDisabled, Models: "flaky-model", Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Ability{Group: "default", Model: "flaky-model", ChannelId: 1}).Error; err != nil {
		t.Fatal(err)
	}

	const userID = 4242 // unique so the process-wide limiter starts clean
	call := func(modelName string) (*httptest.ResponseRecorder, string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("id", userID)
		c.Set("original_model", modelName)
		abortNoChannel(c, "default", modelName, "transient failure")
		if !c.IsAborted() {
			t.Fatal("request must be aborted")
		}
		var body struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Code != "model_not_found" {
			t.Fatalf("code = %q", body.Error.Code)
		}
		return w, body.Error.Message
	}
	countLogs := func() int64 {
		var n int64
		if err := db.Model(&model.Log{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	for range 3 {
		w, message := call("retired-model")
		if w.Code != http.StatusNotFound {
			t.Fatalf("removed model status = %d, want 404 so clients stop retrying", w.Code)
		}
		if !strings.Contains(message, "Model retired-model is not available") || !strings.Contains(message, "https://gw.example.test/pricing") {
			t.Fatalf("unexpected message: %s", message)
		}
	}
	if got := countLogs(); got != 1 {
		t.Fatalf("repeated removed-model requests wrote %d error rows, want 1", got)
	}

	w, message := call("flaky-model")
	if w.Code != http.StatusServiceUnavailable || !strings.HasPrefix(message, "transient failure") {
		t.Fatalf("auto-disabled model must keep the transient 503, got %d %q", w.Code, message)
	}
	if got := countLogs(); got != 2 {
		t.Fatalf("transient failures must still be recorded every time, got %d rows", got)
	}
}
