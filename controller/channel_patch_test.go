package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelPartialUpdatePreservesType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	oldDB, oldLogs, oldRedis, oldMemory := model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = db, db, false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldLogs, oldRedis, oldMemory
		common.SetDatabaseTypes(oldMainType, oldLogType)
		_ = conn.Close()
	})
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	priority, weight := int64(10), uint(20)
	baseURL, setting := "https://fixture.invalid", "{}"
	original := model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Name: "fixture", Key: "synthetic-secret", Status: 1, Models: "fixture-model", Group: "default", BaseURL: &baseURL, Setting: &setting, Priority: &priority, Weight: &weight}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := original.AddAbilities(nil); err != nil {
		t.Fatal(err)
	}
	requestRole := common.RoleRootUser
	call := func(body string) (bool, string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/channel/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", 1)
		c.Set("role", requestRole)
		UpdateChannel(c)
		var result struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Success, result.Message
	}
	for _, body := range []string{`{"id":1,"priority":42}`, `{"id":1,"priority":0}`, `{"id":1,"priority":-1}`, `{"id":1,"weight":0}`, `{"id":1,"weight":7}`, `{"id":1,"type":1,"priority":10}`} {
		if ok, msg := call(body); !ok {
			t.Fatalf("partial update %s failed: %s", body, msg)
		}
		var input struct {
			Priority *int64 `json:"priority"`
			Weight   *uint  `json:"weight"`
		}
		_ = json.Unmarshal([]byte(body), &input)
		var saved model.Channel
		if err := db.First(&saved, 1).Error; err != nil {
			t.Fatal(err)
		}
		if saved.Type != 1 || saved.Key != original.Key || saved.Name != original.Name || saved.Models != original.Models || saved.Group != original.Group || *saved.BaseURL != baseURL || *saved.Setting != setting {
			t.Fatal("unrelated channel fields changed")
		}
		if input.Priority != nil && *saved.Priority != *input.Priority {
			t.Fatal("priority not persisted")
		}
		if input.Weight != nil && *saved.Weight != *input.Weight {
			t.Fatal("weight not persisted")
		}
		var ability model.Ability
		if err := db.Where("channel_id = ?", 1).First(&ability).Error; err != nil {
			t.Fatal(err)
		}
		if *ability.Priority != *saved.Priority || ability.Weight != *saved.Weight {
			t.Fatal("routing abilities not updated")
		}
	}
	for _, body := range []string{`{"id":1,"type":0,"priority":99}`, `{"id":1,"type":null,"priority":99}`, `{"id":1,"type":2,"priority":99}`, `{"id":999,"priority":99}`, `{"id":0,"priority":99}`, `{"id":-1,"priority":99}`} {
		if ok, _ := call(body); ok {
			t.Fatalf("invalid update accepted: %s", body)
		}
	}
	var before model.Channel
	if err := db.First(&before, 1).Error; err != nil {
		t.Fatal(err)
	}
	updated := false
	if err := db.Callback().Update().After("gorm:update").Register("test:mark_channel_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			updated = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_channel_reload", func(tx *gorm.DB) {
		if updated && tx.Statement.Table == "channels" {
			updated = false
			tx.AddError(errors.New("synthetic channel reload failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := call(`{"id":1,"priority":99}`); ok {
		t.Fatal("reload failure reported success")
	}
	if err := db.Callback().Update().Remove("test:mark_channel_update"); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Remove("test:fail_channel_reload"); err != nil {
		t.Fatal(err)
	}
	var reloaded model.Channel
	if err := db.First(&reloaded, 1).Error; err != nil || *reloaded.Priority != *before.Priority {
		t.Fatal("reload failure did not rollback channel")
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_ability", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("synthetic ability insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := call(`{"id":1,"priority":99,"models":"replacement-model"}`); ok {
		t.Fatal("ability failure reported success")
	}
	upstreamEdit := before
	upstreamEdit.Models = "replacement-model"
	if err := updateChannelUpstreamModelSettings(&upstreamEdit, before.GetOtherSettings(), true, before.Models, before.OtherSettings); err == nil {
		t.Fatal("upstream model update hid ability failure")
	}
	if err := db.Callback().Create().Remove("test:fail_ability"); err != nil {
		t.Fatal(err)
	}
	var after model.Channel
	if err := db.First(&after, 1).Error; err != nil {
		t.Fatal(err)
	}
	var abilities []model.Ability
	if err := db.Where("channel_id = ?", 1).Find(&abilities).Error; err != nil {
		t.Fatal(err)
	}
	if after.Models != before.Models || *after.Priority != *before.Priority || len(abilities) != 1 || abilities[0].Model != before.Models || *abilities[0].Priority != *before.Priority {
		t.Fatal("channel or abilities changed after rollback")
	}
	if err := updateChannelUpstreamModelSettings(&upstreamEdit, before.GetOtherSettings(), true, "stale-models", before.OtherSettings); err == nil {
		t.Fatal("stale upstream model result overwrote an intervening edit")
	}
	for _, initialKeys := range []string{"synthetic-a\nsynthetic-b", `["synthetic-a","synthetic-b"]`} {
		info := model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2}
		if err := db.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"key": initialKeys, "channel_info": info}).Error; err != nil {
			t.Fatal(err)
		}
		if ok, msg := call(`{"id":1,"key_mode":"append","key":"synthetic-c\nsynthetic-c"}`); !ok {
			t.Fatal(msg)
		}
		var saved model.Channel
		if err := db.First(&saved, 1).Error; err != nil {
			t.Fatal(err)
		}
		if len(saved.GetKeys()) != 3 || saved.ChannelInfo.MultiKeySize != 3 {
			t.Fatal("multi-key append/deduplicate regression")
		}
		if ok, msg := call(`{"id":1,"key_mode":"replace","key":"synthetic-d"}`); !ok {
			t.Fatal(msg)
		}
		if err := db.First(&saved, 1).Error; err != nil {
			t.Fatal(err)
		}
		if saved.Key != "synthetic-d" || saved.ChannelInfo.MultiKeySize != 1 {
			t.Fatal("multi-key replace regression")
		}
	}
	if ok, msg := call(`{"id":1,"models":"fixture-a,fixture-b","group":"default,fixture"}`); !ok {
		t.Fatal(msg)
	}
	var memberships []model.Ability
	if err := db.Where("channel_id = ?", 1).Find(&memberships).Error; err != nil || len(memberships) != 4 {
		t.Fatal("model/group routing cross-product lost")
	}
	requestRole = common.RoleAdminUser
	if ok, _ := call(`{"id":1,"key":"unauthorized-change"}`); ok {
		t.Fatal("routing admin modified sensitive channel key")
	}
	var protected model.Channel
	if err := db.First(&protected, 1).Error; err != nil || protected.Key != "synthetic-d" {
		t.Fatal("denied key update changed persisted state")
	}
	requestRole = common.RoleRootUser
	if err := db.Model(&model.Channel{}).Where("id = ?", 1).Update("type", 2).Error; err != nil {
		t.Fatal(err)
	}
	if ok, _ := call(`{"id":1,"priority":99}`); ok {
		t.Fatal("omitted type reactivated unsupported channel")
	}
	for _, body := range []string{`{"mode":"single","channel":{"type":2,"key":"synthetic"}}`, `{"mode":"single","channel":{"type":57,"key":"synthetic"}}`, `{"mode":"single","channel":null}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/channel/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role", common.RoleRootUser)
		AddChannel(c)
		var response struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
			t.Fatal("unsupported create accepted")
		}
	}
}

func TestChannelStatusFailureIsNotSuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = oldDB, oldCache })
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Channel{Id: 1, Type: 1, Status: 1, Key: "fixture"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("fixture:reject_ability", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("fixture write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, batch := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := `{"status":2}`
		if batch {
			body = `{"ids":[1],"status":2}`
		}
		c.Request = httptest.NewRequest("POST", "/api/channel/1/status", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		if batch {
			BatchUpdateChannelStatus(c)
		} else {
			UpdateChannelStatus(c)
		}
		var response struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
			t.Fatal("status database failure reported success")
		}
		var saved model.Channel
		if err := db.First(&saved, 1).Error; err != nil || saved.Status != 1 {
			t.Fatal("failed update changed status")
		}
	}
}

func TestChannelConcurrentAppendPostgres(t *testing.T) {
	dsn := os.Getenv("NEWAPI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires an explicitly disposable PostgreSQL fixture")
	}
	if os.Getenv("NEWAPI_TEST_DISPOSABLE") != "true" {
		t.Fatal("refusing database writes without NEWAPI_TEST_DISPOSABLE=true")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(20)
	conn.SetMaxIdleConns(5)
	oldDB, oldLog, oldRedis, oldMemory := model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled
	model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = db, db, false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldLog, oldRedis, oldMemory
	})
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	priority, weight := int64(10), uint(20)
	original := model.Channel{Type: 1, Key: "synthetic-base", Name: "concurrency-fixture", Status: 1, Models: "fixture-model", Group: "default", Priority: &priority, Weight: &weight, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1}}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Where("channel_id = ?", original.Id).Delete(&model.Ability{}); db.Delete(&original) })
	if err := original.AddAbilities(nil); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errors := make(chan string, 12)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			body := fmt.Sprintf(`{"id":%d,"key_mode":"append","key":"synthetic-%d"}`, original.Id, i)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/channel/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", 1)
			c.Set("role", common.RoleRootUser)
			UpdateChannel(c)
			var result struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || !result.Success {
				errors <- "concurrent append failed"
			}
		}(i)
	}
	close(start)
	workers.Wait()
	close(errors)
	for e := range errors {
		t.Error(e)
	}
	var saved model.Channel
	if err := db.First(&saved, original.Id).Error; err != nil {
		t.Fatal(err)
	}
	if len(saved.GetKeys()) != 13 || saved.ChannelInfo.MultiKeySize != 13 {
		t.Fatalf("lost concurrent append: %d keys, metadata size %d", len(saved.GetKeys()), saved.ChannelInfo.MultiKeySize)
	}
	var abilities []model.Ability
	if err := db.Where("channel_id = ?", original.Id).Find(&abilities).Error; err != nil {
		t.Fatal(err)
	}
	if len(abilities) != 1 || *abilities[0].Priority != *saved.Priority || abilities[0].Weight != *saved.Weight {
		t.Fatal("routing drift")
	}
	if conn.Stats().InUse != 0 {
		t.Fatal("leaked connection")
	}
	t.Run("deferred commit failure", func(t *testing.T) {
		// Fail only at COMMIT, after both the channel and abilities were written.
		// This disposable-only constraint must not touch a shared database.
		query := fmt.Sprintf(`CREATE FUNCTION fixture_reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.channel_id = %d THEN RAISE EXCEPTION 'synthetic deferred commit failure'; END IF; RETURN NEW; END $$`, original.Id)
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec("DROP FUNCTION IF EXISTS fixture_reject_commit() CASCADE") })
		if err := db.Exec(`CREATE CONSTRAINT TRIGGER fixture_reject_commit AFTER INSERT OR UPDATE ON abilities DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fixture_reject_commit()`).Error; err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/channel/", strings.NewReader(fmt.Sprintf(`{"id":%d,"priority":987654}`, original.Id)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", 1)
		c.Set("role", common.RoleRootUser)
		UpdateChannel(c)
		var result struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Success {
			t.Fatal("failed commit reported success")
		}
		var after model.Channel
		if err := db.First(&after, original.Id).Error; err != nil {
			t.Fatal(err)
		}
		if after.Key != saved.Key || *after.Priority != *saved.Priority {
			t.Fatal("failed commit changed channel")
		}
		var afterAbilities []model.Ability
		if err := db.Where("channel_id = ?", original.Id).Find(&afterAbilities).Error; err != nil {
			t.Fatal(err)
		}
		if len(afterAbilities) != len(abilities) || *afterAbilities[0].Priority != *abilities[0].Priority {
			t.Fatal("failed commit changed routing")
		}
	})
}
