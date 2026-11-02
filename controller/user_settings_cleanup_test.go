package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	"github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestEmailSettingsPreserveLanguagePrivacyAndHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	oldDB, oldRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() { model.DB, common.RedisEnabled = oldDB, oldRedis; _ = connection.Close() })
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Id: 1, Username: "fixture", Role: common.RoleCommonUser, Quota: 123}
	user.SetSetting(dto.UserSetting{NotifyType: "webhook", WebhookUrl: "https://retired.invalid", SidebarModules: "historical", Language: "vi", RecordIpLog: true, UpstreamModelUpdateNotifyEnabled: true})
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	call := func(handler gin.HandlerFunc, body string) bool {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("id", user.Id)
		handler(c)
		var response struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Success
	}
	for _, method := range []string{"webhook", "bark", "gotify"} {
		if call(UpdateUserSetting, `{"notify_type":"`+method+`","quota_warning_threshold":100}`) {
			t.Fatalf("accepted retired method %s", method)
		}
	}
	if call(UpdateSelf, `{"sidebar_modules":"{}"}`) {
		t.Fatal("accepted personal sidebar write")
	}
	if !call(UpdateUserSetting, `{"notify_type":"email","quota_warning_threshold":100,"notification_email":"alerts@example.invalid","upstream_model_update_notify_enabled":false}`) {
		t.Fatal("email settings failed")
	}
	if err := db.First(&user, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	setting := user.GetSetting()
	if setting.NotifyType != "email" || setting.NotificationEmail != "alerts@example.invalid" || setting.QuotaWarningThreshold != 100 {
		t.Fatal("email settings not persisted")
	}
	if setting.Language != "vi" || !setting.RecordIpLog || setting.SidebarModules != "historical" || setting.WebhookUrl != "https://retired.invalid" || !setting.UpstreamModelUpdateNotifyEnabled || user.Quota != 123 {
		t.Fatal("unrelated user state changed")
	}
	if !call(UpdateUserSetting, `{"notify_type":"email","quota_warning_threshold":100,"record_ip_log":false}`) {
		t.Fatal("privacy update failed")
	}
	if err := db.First(&user, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if user.GetSetting().RecordIpLog || user.GetSetting().NotificationEmail != "" {
		t.Fatal("explicit reset ignored")
	}
}
