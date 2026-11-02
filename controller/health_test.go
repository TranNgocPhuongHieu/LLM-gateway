package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestHealthAndReadiness(t *testing.T) {
	oldDB, oldLog, oldRedis, oldReady := model.DB, model.LOG_DB, common.RedisEnabled, ServingReady.Load()
	oldRDB := common.RDB
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
		ServingReady.Store(oldReady)
		common.RDB = oldRDB
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	probe := func(handler gin.HandlerFunc, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		handler(c)
		if w.Code != want {
			t.Fatalf("got %d, want %d: %s", w.Code, want, w.Body.String())
		}
	}
	ServingReady.Store(false)
	probe(Health, 200)
	probe(Ready, 503)
	ServingReady.Store(true)
	probe(Ready, 200)
	common.RedisEnabled = true
	common.RDB = nil
	probe(Ready, 503)
	common.RDB = oldRDB
	common.RedisEnabled = false
	model.LOG_DB = nil
	probe(Ready, 503)
	model.LOG_DB = db
	_ = conn.Close()
	probe(Ready, 503)
	probe(Health, 200)
}
